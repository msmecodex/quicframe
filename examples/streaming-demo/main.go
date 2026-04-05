// Streaming demo — shows server-push streams and backpressure handling.
//
// Run:
//
//	go run ./examples/streaming-demo
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vmihailenco/msgpack/v5"

	qf "github.com/msmecodex/quicframe/framework"
	"github.com/msmecodex/quicframe/middleware"
	"github.com/msmecodex/quicframe/tlsutil"
)

const chatHistoryLimit = 120

type chatEvent struct {
	ID       string `msgpack:"id"`
	Kind     string `msgpack:"kind"`
	Room     string `msgpack:"room"`
	Sender   string `msgpack:"sender"`
	ClientID string `msgpack:"clientId"`
	Text     string `msgpack:"text"`
	SentAt   int64  `msgpack:"sentAt"`
}

type chatPostBody struct {
	Sender   string `msgpack:"sender"`
	ClientID string `msgpack:"clientId"`
	Text     string `msgpack:"text"`
}

type chatRoom struct {
	history     []chatEvent
	subscribers map[string]chan chatEvent
}

type chatHub struct {
	mu    sync.RWMutex
	rooms map[string]*chatRoom
}

func newChatHub() *chatHub {
	return &chatHub{
		rooms: make(map[string]*chatRoom),
	}
}

func (h *chatHub) subscribe(roomName string) (string, []chatEvent, <-chan chatEvent, func()) {
	roomName = normalizeRoomName(roomName)
	subscriberID := fmt.Sprintf("sub-%d", time.Now().UnixNano())
	ch := make(chan chatEvent, 32)

	h.mu.Lock()
	room := h.ensureRoomLocked(roomName)
	room.subscribers[subscriberID] = ch
	history := slices.Clone(room.history)
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		room := h.rooms[roomName]
		if room != nil {
			if existing, ok := room.subscribers[subscriberID]; ok {
				delete(room.subscribers, subscriberID)
				close(existing)
			}
		}
		h.mu.Unlock()
	}

	return subscriberID, history, ch, unsubscribe
}

func (h *chatHub) publish(roomName string, event chatEvent) {
	roomName = normalizeRoomName(roomName)
	event.Room = roomName

	h.mu.Lock()
	room := h.ensureRoomLocked(roomName)
	room.history = append(room.history, event)
	if len(room.history) > chatHistoryLimit {
		room.history = slices.Clone(room.history[len(room.history)-chatHistoryLimit:])
	}

	subscribers := make([]chan chatEvent, 0, len(room.subscribers))
	for _, subscriber := range room.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	h.mu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (h *chatHub) ensureRoomLocked(roomName string) *chatRoom {
	room := h.rooms[roomName]
	if room != nil {
		return room
	}

	room = &chatRoom{
		history:     make([]chatEvent, 0, chatHistoryLimit),
		subscribers: make(map[string]chan chatEvent),
	}
	h.rooms[roomName] = room
	return room
}

func main() {
	app := qf.New()
	app.Use(middleware.Recovery(), middleware.Logger())
	chat := newChatHub()

	// ── Tick stream: emits a message every 200 ms for up to 60 s ────────────
	app.GET("/stream/ticks", func(c *qf.Context) error {
		sw, err := c.NewStream(200, map[string]string{"x-stream": "ticks"})
		if err != nil {
			return err
		}
		defer sw.Close()

		ticker := time.NewTicker(200 * time.Millisecond)
		deadline := time.After(60 * time.Second)
		defer ticker.Stop()

		for i := 0; ; i++ {
			select {
			case <-deadline:
				return nil
			case t := <-ticker.C:
				payload, err := msgpack.Marshal(map[string]interface{}{
					"seq": i,
					"ts":  t.UnixNano(),
					"msg": fmt.Sprintf("tick #%d", i),
				})
				if err != nil {
					return err
				}
				if _, err := sw.Write(payload); err != nil {
					// Client disconnected — exit cleanly.
					return nil
				}
			}
		}
	})

	// ── Batch stream: emits N items as fast as possible ─────────────────────
	app.GET("/stream/batch/:n", func(c *qf.Context) error {
		n := 100
		fmt.Sscanf(c.Param("n"), "%d", &n)
		if n > 10_000 {
			return c.Error(400, "n must be ≤ 10000")
		}

		sw, err := c.NewStream(200, nil)
		if err != nil {
			return err
		}
		defer sw.Close()

		for i := 0; i < n; i++ {
			data, _ := msgpack.Marshal(map[string]interface{}{
				"index": i,
				"value": i * i,
			})
			if _, err := sw.Write(data); err != nil {
				return nil // client gone
			}
		}
		return nil
	})

	// ── Slow stream: backpressure demo (1 item/s) ────────────────────────────
	app.GET("/stream/slow/:n", func(c *qf.Context) error {
		n := 5
		fmt.Sscanf(c.Param("n"), "%d", &n)
		if n > 60 {
			return c.Error(400, "n must be ≤ 60")
		}

		sw, err := c.NewStream(200, nil)
		if err != nil {
			return err
		}
		defer sw.Close()

		for i := 0; i < n; i++ {
			data, _ := msgpack.Marshal(map[string]interface{}{
				"seq": i,
				"msg": fmt.Sprintf("slow chunk %d/%d", i+1, n),
			})
			if _, err := sw.Write(data); err != nil {
				return nil
			}
			time.Sleep(time.Second) // intentional 1 s delay per chunk
		}
		return nil
	})

	app.GET("/chat/rooms/:room/stream", func(c *qf.Context) error {
		roomName := normalizeRoomName(c.Param("room"))
		subscriberID, history, events, unsubscribe := chat.subscribe(roomName)

		sw, err := c.NewStream(200, map[string]string{"x-stream": "chat-room"})
		if err != nil {
			unsubscribe()
			return err
		}
		defer func() {
			unsubscribe()
			chat.publish(roomName, newSystemEvent(roomName, "A participant left the room."))
			sw.Close()
		}()

		for _, event := range history {
			if err := writeChatEvent(sw, event); err != nil {
				return nil
			}
		}

		chat.publish(roomName, newSystemEvent(roomName, "A participant joined the room."))

		for event := range events {
			if err := writeChatEvent(sw, event); err != nil {
				return nil
			}
		}

		slog.Debug("chat subscriber stream ended", "room", roomName, "subscriber", subscriberID)
		return nil
	})

	app.POST("/chat/rooms/:room/messages", func(c *qf.Context) error {
		roomName := normalizeRoomName(c.Param("room"))

		var body chatPostBody
		if err := c.Bind(&body); err != nil {
			return c.Error(400, "invalid chat payload")
		}

		sender := sanitizeChatName(body.Sender)
		text := strings.TrimSpace(body.Text)
		clientID := strings.TrimSpace(body.ClientID)

		switch {
		case sender == "":
			return c.Error(400, "sender is required")
		case text == "":
			return c.Error(400, "text is required")
		case clientID == "":
			return c.Error(400, "clientId is required")
		}

		event := chatEvent{
			ID:       fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			Kind:     "message",
			Room:     roomName,
			Sender:   sender,
			ClientID: clientID,
			Text:     text,
			SentAt:   time.Now().UnixMilli(),
		}

		chat.publish(roomName, event)

		return c.MsgPack(202, map[string]interface{}{
			"accepted": true,
			"id":       event.ID,
			"room":     roomName,
		})
	})

	// ── Ping ──────────────────────────────────────────────────────────────────
	app.GET("/ping", func(c *qf.Context) error {
		return c.MsgPack(200, map[string]string{"pong": "ok"})
	})

	// ── TLS / start ──────────────────────────────────────────────────────────
	certDir := filepath.Join(".local", "certs", "streaming-demo")
	certFile := filepath.Join(certDir, "localhost-cert.pem")
	keyFile := filepath.Join(certDir, "localhost-key.pem")

	tlsCfg, err := tlsutil.LoadOrCreateSelfSigned(certFile, keyFile, "localhost", "127.0.0.1")
	if err != nil {
		slog.Error("tls", "err", err)
		os.Exit(1)
	}

	certHash, err := tlsutil.CertificateSHA256Hex(tlsCfg)
	if err != nil {
		slog.Error("tls cert hash", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		cancel()
	}()

	slog.Info("streaming demo listening",
		"native", ":4435",
		"webtransport", ":4436",
		"cert_file", certFile,
		"webtransport_cert_sha256", certHash,
		"chat_stream", "/chat/rooms/lounge/stream",
		"chat_post", "/chat/rooms/lounge/messages",
	)
	if err := app.ListenAddr(ctx, ":4435", ":4436", tlsCfg); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func normalizeRoomName(room string) string {
	room = strings.TrimSpace(strings.ToLower(room))
	if room == "" {
		return "lounge"
	}
	return room
}

func sanitizeChatName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if len(name) > 32 {
		return name[:32]
	}
	return name
}

func newSystemEvent(roomName, text string) chatEvent {
	return chatEvent{
		ID:     fmt.Sprintf("sys-%d", time.Now().UnixNano()),
		Kind:   "system",
		Room:   roomName,
		Sender: "system",
		Text:   text,
		SentAt: time.Now().UnixMilli(),
	}
}

func writeChatEvent(sw *qf.StreamWriter, event chatEvent) error {
	payload, err := msgpack.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := sw.Write(payload); err != nil {
		return err
	}
	return nil
}
