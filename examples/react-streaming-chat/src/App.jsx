import { useEffect, useRef, useState } from "react";
import { QuicFrameClient } from "@quicframe/client";

const transportUrl = "https://localhost:4436/wt";
const serverCertificateHashHex = "963118276f5184a6c90067ae52ca4f62833e4cefb3be4449a1e7bf500bee52a5";

function hexToUint8Array(hex) {
  if (!hex) {
    return null;
  }

  const normalized = hex.replace(/\s+/g, "").toLowerCase();
  if (normalized.length % 2 !== 0) {
    throw new Error("Certificate hash must have an even number of hex characters.");
  }

  const bytes = new Uint8Array(normalized.length / 2);
  for (let index = 0; index < normalized.length; index += 2) {
    bytes[index / 2] = Number.parseInt(normalized.slice(index, index + 2), 16);
  }
  return bytes;
}

function createClient() {
  const certHashBytes = hexToUint8Array(serverCertificateHashHex);
  const webTransportOptions = certHashBytes
    ? {
        serverCertificateHashes: [
          {
            algorithm: "sha-256",
            value: certHashBytes
          }
        ]
      }
    : undefined;

  return new QuicFrameClient(transportUrl, {
    webTransportOptions,
    debug: true
  });
}

function normalizeRoomInput(value) {
  const trimmed = value.trim().toLowerCase();
  if (!trimmed) {
    return "lounge";
  }

  return trimmed.replace(/[^a-z0-9-_]+/g, "-").replace(/^-+|-+$/g, "") || "lounge";
}

function randomNickname() {
  return `Guest-${Math.floor(100 + Math.random() * 900)}`;
}

function makeLocalNotice(content) {
  return {
    id: crypto.randomUUID(),
    kind: "local",
    sender: "system",
    text: content,
    sentAt: Date.now()
  };
}

function formatTime(timestamp) {
  if (!timestamp) {
    return "";
  }

  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit"
  }).format(new Date(timestamp));
}

export default function App() {
  const clientRef = useRef(null);
  const feedRef = useRef(null);
  const clientIdRef = useRef(crypto.randomUUID());
  const [connected, setConnected] = useState(false);
  const [status, setStatus] = useState("Connecting to room stream...");
  const [error, setError] = useState("");
  const [sending, setSending] = useState(false);
  const [roomInput, setRoomInput] = useState("lounge");
  const [room, setRoom] = useState("lounge");
  const [nickname, setNickname] = useState(randomNickname);
  const [composer, setComposer] = useState("");
  const [messages, setMessages] = useState([
    makeLocalNotice("Open this same page in another browser, keep the same room name, and chat live.")
  ]);

  useEffect(() => {
    const feed = feedRef.current;
    if (!feed) {
      return;
    }

    feed.scrollTo({
      top: feed.scrollHeight,
      behavior: "smooth"
    });
  }, [messages]);

  useEffect(() => {
    const client = createClient();
    clientRef.current = client;
    let active = true;

    setConnected(false);
    setError("");
    setMessages([
      makeLocalNotice(`Joining room "${room}" and waiting for live messages...`)
    ]);

    async function connectAndStream() {
      try {
        await client.connect();
        if (!active) {
          return;
        }

        setConnected(true);
        setStatus(`Connected to room "${room}". Share this room name in another browser.`);

        const stream = await client.stream("GET", `/chat/rooms/${room}/stream`);
        for await (const event of stream) {
          if (!active || !event || typeof event !== "object") {
            continue;
          }

          setMessages((currentMessages) => {
            const alreadyExists = currentMessages.some((message) => message.id === event.id);
            if (alreadyExists) {
              return currentMessages;
            }
            return [...currentMessages, event];
          });
        }

        if (active) {
          setStatus(`Disconnected from room "${room}".`);
          setConnected(false);
        }
      } catch (err) {
        if (!active) {
          return;
        }

        setConnected(false);
        setStatus(`Unable to stream room "${room}".`);
        setError(
          err instanceof Error ? err.message : "Unable to connect to the room stream."
        );
      }
    }

    void connectAndStream();

    return () => {
      active = false;
      client.close().catch(() => {});
    };
  }, [room]);

  async function handleSendMessage(event) {
    event.preventDefault();
    if (!clientRef.current || !connected || sending) {
      return;
    }

    const text = composer.trim();
    const sender = nickname.trim();
    if (!text || !sender) {
      return;
    }

    setSending(true);
    setError("");

    try {
      await clientRef.current.post(`/chat/rooms/${room}/messages`, {
        sender,
        clientId: clientIdRef.current,
        text
      });
      setComposer("");
      setStatus(`Message sent to room "${room}".`);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Unable to send the message."
      );
    } finally {
      setSending(false);
    }
  }

  function handleJoinRoom(event) {
    event.preventDefault();
    const nextRoom = normalizeRoomInput(roomInput);
    setRoomInput(nextRoom);

    if (nextRoom === room) {
      setStatus(`Already connected to room "${room}".`);
      return;
    }

    setRoom(nextRoom);
  }

  return (
    <main className="page">
      <section className="hero">
        <div>
          <p className="eyebrow">QuicFrame x React</p>
          <h1>Browser-to-browser room chat</h1>
          <p className="intro">
            Open this same link in two different browsers, join the same room,
            and each message will stream to every connected participant over WebTransport.
          </p>
        </div>

        <article className="status-card">
          <div className="status-row">
            <span className="status-label">Connection</span>
            <span className={connected ? "badge online" : "badge offline"}>
              {connected ? "connected" : "offline"}
            </span>
          </div>
          <p>{status}</p>
          <p className="muted">Transport URL: {transportUrl}</p>
          <p className="muted">Current room: {room}</p>
          <p className="muted">
            Keep the same <code>serverCertificateHashHex</code> in every browser tab or window.
          </p>
          {error ? <p className="error">{error}</p> : null}
        </article>
      </section>

      <section className="layout">
        <aside className="panel sidebar">
          <h2>Session</h2>

          <form className="stack-form" onSubmit={handleJoinRoom}>
            <label>
              Display name
              <input
                value={nickname}
                onChange={(event) => setNickname(event.target.value)}
                placeholder="Guest-101"
                maxLength="32"
              />
            </label>

            <label>
              Room name
              <input
                value={roomInput}
                onChange={(event) => setRoomInput(event.target.value)}
                placeholder="lounge"
              />
            </label>

            <button type="submit">Join room</button>
          </form>

          <div className="code-card">
            <p className="code-label">How to test</p>
            <pre>{`1. Run the streaming demo server.
2. Open this page in two browsers.
3. Join the same room in both.
4. Send a message from either side.`}</pre>
          </div>
        </aside>

        <section className="panel chat-shell">
          <div className="chat-header">
            <div>
              <h2>Room: {room}</h2>
              <p className="muted">Messages are broadcast to every connected browser in this room.</p>
            </div>
            <span className={sending ? "badge live" : "badge idle"}>
              {sending ? "sending" : "live stream"}
            </span>
          </div>

          <div ref={feedRef} className="messages">
            {messages.map((message) => {
              const isSelf = message.clientId === clientIdRef.current;
              const kind = message.kind || "message";
              const className =
                kind === "system" || kind === "local"
                  ? "message system"
                  : isSelf
                    ? "message user"
                    : "message assistant";

              return (
                <article key={message.id} className={className}>
                  <div className="message-meta">
                    <span className="message-role">
                      {kind === "system" || kind === "local" ? "room" : message.sender}
                    </span>
                    <span className="timestamp">{formatTime(message.sentAt)}</span>
                  </div>
                  <p>{message.text}</p>
                </article>
              );
            })}
          </div>

          <form className="composer" onSubmit={handleSendMessage}>
            <label className="composer-field">
              <span className="sr-only">Message</span>
              <textarea
                rows="3"
                value={composer}
                onChange={(event) => setComposer(event.target.value)}
                placeholder={`Message #${room} as ${nickname || "yourself"}...`}
                disabled={!connected || sending}
              />
            </label>
            <button type="submit" disabled={!connected || sending || !composer.trim() || !nickname.trim()}>
              {sending ? "Sending..." : "Send"}
            </button>
          </form>
        </section>
      </section>
    </main>
  );
}
