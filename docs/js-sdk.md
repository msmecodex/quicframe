# JavaScript SDK

The JavaScript SDK lives in `sdk/js` and targets browsers using WebTransport, with a fallback to `fetch` when WebTransport is unavailable.

Package metadata:

- package name: `@quicframe/client`
- module type: ESM
- repository: `https://github.com/msmecodex/quicframe/`

## Features

- WebTransport client for QUIC-native requests
- automatic `fetch` fallback
- MsgPack request and response handling
- async-iterable stream consumption
- ping helper for latency checks

## Install

If the package is published to npm:

```bash
npm install @quicframe/client
```

If you are working from this repository directly:

```bash
git clone https://github.com/msmecodex/quicframe/
cd quicframe/sdk/js
npm install
npm run build
```

The built package is emitted to `dist/`.

## Create a Client

```js
import { QuicFrameClient } from "@quicframe/client";

const client = new QuicFrameClient("https://localhost:4434/wt", {
  defaultHeaders: {
    authorization: "Bearer token",
  },
  fallbackBase: "https://localhost:4434",
});

await client.connect();
```

When consuming from npm, the package resolves from `dist/` automatically through `package.json` exports.

## Unary Requests

```js
const resp = await client.get("/ping");
console.log(resp.status);
console.log(resp.decode());
```

POST, PUT, and PATCH will MsgPack-encode normal JavaScript objects for you.

```js
const resp = await client.post("/users", { name: "Alice" });
```

## Fallback Mode

If WebTransport is not available or the connection fails, the client switches to `fetch` mode.

That is useful when:

- the browser lacks WebTransport support
- the page is not running in a usable secure context
- you want a softer migration path for browser clients

## Streaming

Streaming requires WebTransport. It is not available in fetch fallback mode.

```js
const stream = await client.stream("GET", "/events");

for await (const item of stream) {
  console.log(item);
}
```

The stream yields decoded MsgPack objects, not raw frames.

## Ping

```js
const ms = await client.ping();
console.log(`latency=${ms}ms`);
```

Ping also requires WebTransport.

## Response Object

`QfResponse` exposes:

- `status`
- `headers`
- `decode()`
- `rawBody`
- `ok`

## Browser Requirements

- HTTPS is required for WebTransport in normal browser use.
- Certificates must be accepted by the browser.
- If you use self-signed certs locally, you may need to trust them manually.

## Protocol Helpers

The SDK also exports protocol helpers from `@quicframe/client/protocol` for lower-level integrations.
