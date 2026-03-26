/**
 * QuicFrame binary protocol codec — browser-side.
 *
 * Wire format (identical to the Go server):
 *
 *   ┌──────────────────┐
 *   │  FrameLen  (4B)  │  ← big-endian Uint32: byte count of Type + Payload
 *   ├──────────────────┤
 *   │  FrameType (1B)  │
 *   ├──────────────────┤
 *   │  Payload (msgpack│  ← @msgpack/msgpack encoded object
 *   └──────────────────┘
 *
 * Serialisation: MsgPack exclusively — never JSON.
 */

import { encode, decode } from '@msgpack/msgpack';

// ── Frame type discriminants ─────────────────────────────────────────────────
export const FRAME_REQUEST     = 0x01;
export const FRAME_RESPONSE    = 0x02;
export const FRAME_STREAM_DATA = 0x03;
export const FRAME_STREAM_END  = 0x04;
export const FRAME_ERROR       = 0x05;
export const FRAME_PING        = 0x06;
export const FRAME_PONG        = 0x07;

/** Maximum single frame size (64 MiB). */
export const MAX_FRAME_SIZE = 64 * 1024 * 1024;

// ── Encoder ──────────────────────────────────────────────────────────────────

/**
 * Encode a payload as a length-prefixed QuicFrame wire frame.
 *
 * @param {number} frameType
 * @param {object} payload
 * @returns {Uint8Array}
 */
export function encodeFrame(frameType, payload) {
  const encoded = encode(payload);                         // msgpack
  const frameLen = 1 + encoded.byteLength;

  if (frameLen > MAX_FRAME_SIZE) {
    throw new Error(`quicframe: payload too large (${frameLen} bytes)`);
  }

  const buf  = new ArrayBuffer(4 + frameLen);
  const view = new DataView(buf);
  view.setUint32(0, frameLen, false);   // big-endian length
  view.setUint8(4, frameType);          // type byte
  new Uint8Array(buf, 5).set(encoded);  // msgpack payload

  return new Uint8Array(buf);
}

// ── Decoder ──────────────────────────────────────────────────────────────────

/**
 * Stateful byte-buffer that accumulates raw bytes from a ReadableStream
 * and allows extracting complete frames.
 */
export class FrameBuffer {
  constructor() {
    /** @type {Uint8Array} */
    this._buf = new Uint8Array(0);
  }

  /** Append a new chunk from the stream reader. */
  push(chunk) {
    const merged = new Uint8Array(this._buf.byteLength + chunk.byteLength);
    merged.set(this._buf);
    merged.set(chunk, this._buf.byteLength);
    this._buf = merged;
  }

  /**
   * Try to extract one complete frame.
   * Returns `null` when not enough bytes have been buffered yet.
   *
   * @returns {{ frameType: number, payload: any } | null}
   */
  tryRead() {
    if (this._buf.byteLength < 4) return null;

    const view      = new DataView(this._buf.buffer, this._buf.byteOffset);
    const frameLen  = view.getUint32(0, false);           // big-endian
    const totalSize = 4 + frameLen;

    if (this._buf.byteLength < totalSize) return null;    // incomplete

    const frameType = this._buf[4];
    const msgpackPayload = this._buf.slice(5, totalSize);
    this._buf = this._buf.slice(totalSize);               // advance buffer

    return { frameType, payload: decode(msgpackPayload) };
  }
}

// ── Convenience constructors ─────────────────────────────────────────────────

/** @returns {Uint8Array} */
export function encodeRequest({ id, method, path, headers = {}, body = new Uint8Array(0), stream = false }) {
  return encodeFrame(FRAME_REQUEST, { id, method, path, headers, body, stream });
}

/** @returns {Uint8Array} */
export function encodePing() {
  return encodeFrame(FRAME_PING, { ts: Date.now() * 1_000_000 }); // nanoseconds
}
