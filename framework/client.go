package quicframe

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/msmecodex/quicframe/protocol"
	"github.com/quic-go/quic-go"
	"github.com/vmihailenco/msgpack/v5"
)

type Client struct {
	conn    *quic.Conn
	quicCfg *quic.Config
	mu      sync.RWMutex
	hashes  []string
}

// NewClient creates a new QuicFrame client and dials the server.
func Dial(ctx context.Context, addr string, tlsCfg *tls.Config, quicCfg *quic.Config) (*Client, error) {
	if quicCfg == nil {
		quicCfg = &quic.Config{
			KeepAlivePeriod: 30 * time.Second,
			MaxIdleTimeout:  5 * time.Minute,
			EnableDatagrams: true,
		}
	}

	cfg := tlsCfg.Clone()
	cfg.NextProtos = []string{"qf/1"}

	conn, err := quic.DialAddr(ctx, addr, cfg, quicCfg)
	if err != nil {
		return nil, fmt.Errorf("quicframe: dial: %w", err)
	}

	return &Client{
		conn:    conn,
		quicCfg: quicCfg,
	}, nil
}

// Request sends a single request and returns the response.
func (c *Client) Request(ctx context.Context, method, path string, body interface{}) (*protocol.Response, error) {
	return c.RequestWithHeaders(ctx, method, path, nil, body)
}

// RequestWithHeaders sends a request with optional headers and returns the response.
func (c *Client) RequestWithHeaders(ctx context.Context, method, path string, headers map[string]string, body interface{}) (*protocol.Response, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	req := &protocol.Request{
		ID:      fmt.Sprintf("req-%d", time.Now().UnixNano()),
		Method:  method,
		Path:    path,
		Headers: headers,
	}

	if body != nil {
		if b, ok := body.([]byte); ok {
			req.Body = b
		} else {
			b, err := msgpack.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("quicframe: marshal body: %w", err)
			}
			req.Body = b
		}
	}

	if err := protocol.WriteFrame(stream, protocol.FrameTypeRequest, req); err != nil {
		return nil, err
	}

	ft, data, err := protocol.ReadFrame(stream)
	if err != nil {
		return nil, err
	}

	if ft == protocol.FrameTypeError {
		errFrame, _ := protocol.DecodeError(data)
		return nil, fmt.Errorf("quicframe: server error %d: %s", errFrame.Code, errFrame.Message)
	}

	if ft != protocol.FrameTypeResponse {
		return nil, fmt.Errorf("unexpected frame type: %v", ft)
	}

	return protocol.DecodeResponse(data)
}

// WatchCertFile watches a certificate file and updates pinned hashes.
func (c *Client) WatchCertFile(certFile string) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		lastHash := ""
		for range ticker.C {
			hash, err := calculateCertHash(certFile)
			if err == nil && hash != lastHash {
				c.mu.Lock()
				c.hashes = []string{hash}
				c.mu.Unlock()
				lastHash = hash
			}
		}
	}()
}

func calculateCertHash(certFile string) (string, error) {
	data, err := os.ReadFile(certFile)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	return c.conn.CloseWithError(0, "client closed")
}
