/**
 * QuicFrame browser client — Axios-style API over WebTransport.
 *
 * Falls back to `fetch` (HTTP/1.1 or HTTP/2 with MsgPack body encoding) when
 * the WebTransport API is unavailable (e.g. non-HTTPS origins, old browsers).
 *
 * Usage:
 *
 *   import { QuicFrameClient } from '@quicframe/client';
 *
 *   const client = new QuicFrameClient('https://api.example.com:4434/wt');
 *   await client.connect();
 *
 *   const resp = await client.get('/users');
 *   const users = resp.decode();   // rmp-decode body → plain object
 *
 *   const stream = await client.stream('GET', '/events');
 *   for await (const chunk of stream) {
 *     console.log(chunk);
 *   }
 */

import { encode, decode } from '@msgpack/msgpack';
import {
  encodeRequest, encodePing, FrameBuffer,
  FRAME_RESPONSE, FRAME_STREAM_DATA, FRAME_STREAM_END,
  FRAME_ERROR, FRAME_PONG,
} from './protocol.js';

// ── Response wrapper ─────────────────────────────────────────────────────────

class QfResponse {
  /**
   * @param {number} status
   * @param {Record<string,string>} headers
   * @param {Uint8Array} body  raw MsgPack bytes
   */
  constructor(status, headers, body) {
    this.status  = status;
    this.headers = headers;
    this._body   = body;
  }

  /** Decode the MsgPack body into a plain JavaScript value. */
  decode() {
    if (!this._body || this._body.byteLength === 0) return null;
    return decode(this._body);
  }

  /** Returns the raw MsgPack bytes. */
  get rawBody() { return this._body; }

  get ok() { return this.status >= 200 && this.status < 300; }
}

// ── AsyncStream ──────────────────────────────────────────────────────────────

/**
 * Async iterable that yields decoded MsgPack objects for each stream chunk.
 */
class QfStream {
  /**
   * @param {ReadableStreamDefaultReader} reader  raw bytes reader
   */
  constructor(reader) {
    this._reader = reader;
    this._buf    = new FrameBuffer();
    this._done   = false;
  }

  [Symbol.asyncIterator]() { return this; }

  async next() {
    if (this._done) return { value: undefined, done: true };

    while (true) {
      const frame = this._buf.tryRead();
      if (frame) {
        switch (frame.frameType) {
          case FRAME_STREAM_DATA:
            if (frame.payload.final) {
              this._done = true;
              return { value: undefined, done: true };
            }
            return { value: decode(frame.payload.data), done: false };
          case FRAME_STREAM_END:
            this._done = true;
            return { value: undefined, done: true };
          case FRAME_ERROR:
            this._done = true;
            throw new QuicFrameError(frame.payload.code, frame.payload.message);
          default:
            // skip unknown frames
        }
      }

      const { value, done } = await this._reader.read();
      if (done) {
        this._done = true;
        return { value: undefined, done: true };
      }
      this._buf.push(value);
    }
  }
}

// ── Error type ───────────────────────────────────────────────────────────────

export class QuicFrameError extends Error {
  constructor(code, message) {
    super(message);
    this.name = 'QuicFrameError';
    this.code = code;
  }
}

// ── Client ───────────────────────────────────────────────────────────────────

export class QuicFrameClient {
  /**
   * @param {string} url  WebTransport URL, e.g. "https://api.example.com:4434/wt"
   * @param {object} [opts]
   * @param {Record<string,string>} [opts.defaultHeaders]  merged into every request
   * @param {string} [opts.fallbackBase]  HTTP base URL for the fetch fallback
   */
  constructor(url, opts = {}) {
    this._url            = url;
    this._defaultHeaders = opts.defaultHeaders ?? {};
    this._fallbackBase   = opts.fallbackBase  ?? null;
    this._transport      = null;   // WebTransport instance
    this._useFallback    = false;
  }

  // ── Lifecycle ──────────────────────────────────────────────────────────────

  /** Establish the WebTransport connection. Falls back to fetch on failure. */
  async connect() {
    if (typeof WebTransport === 'undefined') {
      console.warn('[quicframe] WebTransport not available – falling back to fetch');
      this._useFallback = true;
      return;
    }

    try {
      this._transport = new WebTransport(this._url);
      await this._transport.ready;
      console.info('[quicframe] WebTransport connected');
    } catch (err) {
      console.warn('[quicframe] WebTransport connection failed, falling back to fetch:', err);
      this._useFallback = true;
    }
  }

  /** Close the WebTransport session. */
  async close() {
    if (this._transport) {
      this._transport.close();
      this._transport = null;
    }
  }

  // ── Request methods ────────────────────────────────────────────────────────

  /** Issue a GET request. Returns a {@link QfResponse}. */
  async get(path, headers = {}) {
    return this.request('GET', path, headers, null);
  }

  /** Issue a POST request with a MsgPack-encoded body. */
  async post(path, body, headers = {}) {
    return this.request('POST', path, headers, body);
  }

  /** Issue a PUT request with a MsgPack-encoded body. */
  async put(path, body, headers = {}) {
    return this.request('PUT', path, headers, body);
  }

  /** Issue a DELETE request. */
  async delete(path, headers = {}) {
    return this.request('DELETE', path, headers, null);
  }

  /** Issue a PATCH request. */
  async patch(path, body, headers = {}) {
    return this.request('PATCH', path, headers, body);
  }

  /**
   * Core request dispatcher.  Uses WebTransport if connected, otherwise
   * falls back to `fetch` with `application/x-msgpack` content-type.
   *
   * @param {string} method
   * @param {string} path
   * @param {Record<string,string>} headers
   * @param {any} [body]  will be MsgPack-encoded if not already Uint8Array
   * @returns {Promise<QfResponse>}
   */
  async request(method, path, headers = {}, body = null) {
    const merged = { ...this._defaultHeaders, ...headers };

    if (!this._useFallback && this._transport) {
      return this._wtRequest(method, path, merged, body);
    }
    return this._fetchFallback(method, path, merged, body);
  }

  // ── Streaming request ──────────────────────────────────────────────────────

  /**
   * Open a streaming request.  Returns an async iterable of decoded objects.
   *
   * ```js
   * const stream = await client.stream('GET', '/events');
   * for await (const event of stream) { console.log(event); }
   * ```
   *
   * @returns {Promise<QfStream>}
   */
  async stream(method, path, headers = {}, body = null) {
    if (this._useFallback || !this._transport) {
      throw new QuicFrameError(501, 'Streaming requires WebTransport');
    }

    const merged   = { ...this._defaultHeaders, ...headers };
    const bodyBytes = body != null
      ? (body instanceof Uint8Array ? body : encode(body))
      : new Uint8Array(0);

    const biStream = await this._transport.createBidirectionalStream();
    const writer   = biStream.writable.getWriter();

    const requestId = crypto.randomUUID();
    const frame = encodeRequest({
      id: requestId, method, path, headers: merged,
      body: bodyBytes, stream: true,
    });
    await writer.write(frame);
    await writer.close();

    const reader = biStream.readable.getReader();

    // Read and discard the initial FRAME_RESPONSE header.
    const buf = new FrameBuffer();
    while (true) {
      const { value, done } = await reader.read();
      if (done) throw new QuicFrameError(500, 'stream closed before response header');
      buf.push(value);
      const f = buf.tryRead();
      if (f) {
        if (f.frameType === FRAME_ERROR) {
          throw new QuicFrameError(f.payload.code, f.payload.message);
        }
        if (f.frameType === FRAME_RESPONSE) break; // header received
      }
    }

    return new QfStream(reader);
  }

  // ── Ping ───────────────────────────────────────────────────────────────────

  /** Measure round-trip latency.  Returns milliseconds. */
  async ping() {
    if (this._useFallback || !this._transport) {
      throw new QuicFrameError(501, 'Ping requires WebTransport');
    }

    const start    = performance.now();
    const biStream = await this._transport.createBidirectionalStream();
    const writer   = biStream.writable.getWriter();

    await writer.write(encodePing());
    await writer.close();

    const reader = biStream.readable.getReader();
    const buf    = new FrameBuffer();

    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buf.push(value);
      const f = buf.tryRead();
      if (f && f.frameType === FRAME_PONG) break;
    }

    return performance.now() - start;
  }

  // ── Internal: WebTransport request ────────────────────────────────────────

  async _wtRequest(method, path, headers, body) {
    const bodyBytes = body != null
      ? (body instanceof Uint8Array ? body : encode(body))
      : new Uint8Array(0);

    const biStream = await this._transport.createBidirectionalStream();
    const writer   = biStream.writable.getWriter();

    const requestId = crypto.randomUUID();
    const frame = encodeRequest({
      id: requestId, method, path, headers, body: bodyBytes,
    });
    await writer.write(frame);
    await writer.close();

    const reader = biStream.readable.getReader();
    const buf    = new FrameBuffer();

    while (true) {
      const { value, done } = await reader.read();
      if (done) throw new QuicFrameError(500, 'stream closed before response');
      buf.push(value);
      const f = buf.tryRead();
      if (!f) continue;

      switch (f.frameType) {
        case FRAME_RESPONSE:
          return new QfResponse(f.payload.status, f.payload.headers ?? {}, f.payload.body ?? new Uint8Array(0));
        case FRAME_ERROR:
          throw new QuicFrameError(f.payload.code, f.payload.message);
        default:
          // Ignore unexpected frames; keep reading.
      }
    }

    throw new QuicFrameError(500, 'no response received');
  }

  // ── Internal: fetch fallback ───────────────────────────────────────────────

  async _fetchFallback(method, path, headers, body) {
    if (!this._fallbackBase) {
      throw new QuicFrameError(
        503,
        'WebTransport unavailable and no fallbackBase configured'
      );
    }

    const url = this._fallbackBase.replace(/\/$/, '') + path;
    const fetchHeaders = {
      'accept':       'application/x-msgpack',
      'content-type': 'application/x-msgpack',
      ...headers,
    };

    const fetchOpts = {
      method,
      headers: fetchHeaders,
    };

    if (body != null && method !== 'GET' && method !== 'HEAD') {
      fetchOpts.body = body instanceof Uint8Array ? body : encode(body);
    }

    const res = await fetch(url, fetchOpts);
    const rawBody = new Uint8Array(await res.arrayBuffer());

    return new QfResponse(res.status, Object.fromEntries(res.headers.entries()), rawBody);
  }
}

export { QfResponse };
