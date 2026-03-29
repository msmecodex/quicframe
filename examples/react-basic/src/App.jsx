import { useEffect, useRef, useState } from "react";
import { QuicFrameClient } from "@quicframe/client";

const transportUrl = "https://localhost:4434/wt";
const serverCertificateHashHex = "7b5dfb59d1345491f1781b38e2735c3391e80d2dbc508fff55501dc2170269d9";

function hexToUint8Array(hex) {
  if (!hex) {
    return null;
  }

  const normalized = hex.replace(/\s+/g, "").toLowerCase();
  if (normalized.length % 2 !== 0) {
    throw new Error("Certificate hash must have an even number of hex characters.");
  }

  const bytes = new Uint8Array(normalized.length / 2);
  for (let i = 0; i < normalized.length; i += 2) {
    bytes[i / 2] = Number.parseInt(normalized.slice(i, i + 2), 16);
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

function decodeResponse(response) {
  const payload = response.decode();
  return payload && typeof payload === "object" ? payload : null;
}

function logUiEvent(event, data) {
  console.debug(`[react-basic] ${event}`, data);
}

export default function App() {
  const clientRef = useRef(null);
  const [status, setStatus] = useState("Connecting to QuicFrame server...");
  const [connected, setConnected] = useState(false);
  const [pingData, setPingData] = useState(null);
  const [users, setUsers] = useState([]);
  const [form, setForm] = useState({ name: "", role: "viewer" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const client = createClient();
    clientRef.current = client;

    let active = true;

    async function bootstrap() {
      try {
        await client.connect();
        if (!active) {
          return;
        }

        setConnected(true);
        setStatus("Connected. Loading /ping and /users...");

        await Promise.all([loadPing(client), loadUsers(client)]);

        if (active) {
          setStatus("Connected to basic server over the JS SDK.");
        }
      } catch (err) {
        if (!active) {
          return;
        }
        setConnected(false);
        setStatus("Connection failed.");
        setError(
          err instanceof Error ? err.message : "Unable to connect to QuicFrame server."
        );
      }
    }

    bootstrap();

    return () => {
      active = false;
      client.close().catch(() => {});
    };
  }, []);

  function handleAsyncError(err) {
    setError(
      err instanceof Error ? err.message : "Request failed."
    );
  }

  async function loadPing(client = clientRef.current) {
    logUiEvent("request", { method: "GET", path: "/ping" });
    const response = await client.get("/ping");
    const payload = decodeResponse(response);
    logUiEvent("response", { method: "GET", path: "/ping", status: response.status, payload });
    setPingData({
      status: response.status,
      body: payload
    });
  }

  async function loadUsers(client = clientRef.current) {
    logUiEvent("request", { method: "GET", path: "/users" });
    const response = await client.get("/users");
    const payload = decodeResponse(response);
    logUiEvent("response", { method: "GET", path: "/users", status: response.status, payload });
    setUsers(Array.isArray(payload?.users) ? payload.users : []);
  }

  async function handleCreateUser(event) {
    event.preventDefault();
    if (!clientRef.current) {
      return;
    }

    setSubmitting(true);
    setError("");

    try {
      logUiEvent("request", { method: "POST", path: "/users", body: form });
      const response = await clientRef.current.post("/users", form);
      const createdUser = decodeResponse(response);
      logUiEvent("response", { method: "POST", path: "/users", status: response.status, payload: createdUser });

      if (createdUser) {
        setUsers((currentUsers) => [...currentUsers, createdUser]);
      }

      setForm({ name: "", role: "viewer" });
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to create the user."
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="page">
      <section className="hero">
        <p className="eyebrow">QuicFrame x React</p>
        <h1>Basic server API calls through the JS SDK</h1>
        <p className="intro">
          This example connects directly to the QuicFrame basic server, calls
          <code> /ping </code>
          and
          <code> /users </code>
          on load, then creates a new user with
          <code> client.post("/users") </code>.
        </p>
      </section>

      <section className="panel">
        <div className="panel-header">
          <h2>Connection</h2>
          <span className={connected ? "badge online" : "badge offline"}>
            {connected ? "connected" : "offline"}
          </span>
        </div>
        <p>{status}</p>
        <p className="muted">Transport URL: {transportUrl}</p>
        <p className="muted">
          Set <code>serverCertificateHashHex</code> in this file for self-signed
          local WebTransport.
        </p>
        {error ? <p className="error">{error}</p> : null}
      </section>

      <section className="grid">
        <article className="panel">
          <div className="panel-header">
            <h2>/ping response</h2>
            <button
              type="button"
              onClick={() => {
                void loadPing().catch(handleAsyncError);
              }}
              disabled={!connected}
            >
              Refresh
            </button>
          </div>
          <pre>{JSON.stringify(pingData, null, 2)}</pre>
        </article>

        <article className="panel">
          <div className="panel-header">
            <h2>/users response</h2>
            <button
              type="button"
              onClick={() => {
                void loadUsers().catch(handleAsyncError);
              }}
              disabled={!connected}
            >
              Refresh
            </button>
          </div>
          <ul className="users">
            {users.map((user) => (
              <li key={user.id}>
                <strong>{user.name}</strong>
                <span>ID: {user.id}</span>
              </li>
            ))}
          </ul>
        </article>
      </section>

      <section className="panel">
        <div className="panel-header">
          <h2>Create user</h2>
          <span className="muted">POST /users</span>
        </div>
        <form className="form" onSubmit={handleCreateUser}>
          <label>
            Name
            <input
              value={form.name}
              onChange={(event) =>
                setForm((current) => ({ ...current, name: event.target.value }))
              }
              placeholder="Alice"
              required
            />
          </label>

          <label>
            Role
            <select
              value={form.role}
              onChange={(event) =>
                setForm((current) => ({ ...current, role: event.target.value }))
              }
            >
              <option value="viewer">viewer</option>
              <option value="editor">editor</option>
              <option value="admin">admin</option>
            </select>
          </label>

          <button type="submit" disabled={!connected || submitting}>
            {submitting ? "Creating..." : "Create user"}
          </button>
        </form>
      </section>
    </main>
  );
}
