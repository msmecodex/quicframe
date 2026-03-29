//! Wire-level frame codec — mirrors the Go server's `protocol` package.
//!
//! Frame wire format:
//! ```text
//! ┌──────────────────┐
//! │  FrameLen  (4B)  │  ← big-endian u32: byte count of Type + Payload
//! ├──────────────────┤
//! │  FrameType (1B)  │
//! ├──────────────────┤
//! │  Payload (msgpack│  ← rmp_serde-encoded struct
//! └──────────────────┘
//! ```

use std::collections::HashMap;

use bytes::{BufMut, BytesMut};
use serde::{Deserialize, Serialize};
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};

use crate::error::QfError;

// ── Frame type discriminants ─────────────────────────────────────────────────

pub const FRAME_REQUEST: u8 = 0x01;
pub const FRAME_RESPONSE: u8 = 0x02;
pub const FRAME_STREAM_DATA: u8 = 0x03;
pub const FRAME_STREAM_END: u8 = 0x04;
pub const FRAME_ERROR: u8 = 0x05;
pub const FRAME_PING: u8 = 0x06;
pub const FRAME_PONG: u8 = 0x07;

/// Maximum single frame size (64 MiB).
pub const MAX_FRAME_SIZE: u32 = 64 * 1024 * 1024;

// ── Payload types ────────────────────────────────────────────────────────────

/// Outgoing request (client → server).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Request {
    pub id: String,
    pub method: String,
    pub path: String,
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub headers: HashMap<String, String>,
    #[serde(default, with = "serde_bytes", skip_serializing_if = "Vec::is_empty")]
    pub body: Vec<u8>,
    #[serde(default, skip_serializing_if = "std::ops::Not::not")]
    pub stream: bool,
}

/// Incoming response (server → client).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Response {
    pub id: String,
    pub status: i32,
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub headers: HashMap<String, String>,
    #[serde(default, with = "serde_bytes", skip_serializing_if = "Vec::is_empty")]
    pub body: Vec<u8>,
    /// true when FrameStreamData/FrameStreamEnd frames follow.
    #[serde(default)]
    pub stream: bool,
}

/// A chunk in a streaming response.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StreamChunk {
    pub id: String,
    pub seq: u64,
    #[serde(default, with = "serde_bytes", skip_serializing_if = "Vec::is_empty")]
    pub data: Vec<u8>,
    pub r#final: bool,
}

/// Protocol-level error.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErrorFrame {
    #[serde(default)]
    pub id: String,
    pub code: i32,
    pub message: String,
}

/// Keep-alive ping/pong.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PingFrame {
    pub ts: i64,
}

// ── Codec ────────────────────────────────────────────────────────────────────

/// Write a length-prefixed frame to an async writer.
///
/// ```text
/// [4B BE u32 frameLen][1B frameType][msgpack(payload)]
/// ```
pub async fn write_frame<W, T>(w: &mut W, frame_type: u8, payload: &T) -> Result<(), QfError>
where
    W: AsyncWrite + Unpin,
    T: Serialize,
{
    let encoded = rmp_serde::to_vec_named(payload)
        .map_err(|e| QfError::Codec(format!("msgpack encode: {e}")))?;

    let frame_len = 1u32 + encoded.len() as u32;
    if frame_len > MAX_FRAME_SIZE {
        return Err(QfError::Codec(format!(
            "payload too large: {frame_len} bytes"
        )));
    }

    let mut buf = BytesMut::with_capacity(4 + 1 + encoded.len());
    buf.put_u32(frame_len); // big-endian length
    buf.put_u8(frame_type); // type discriminant
    buf.extend_from_slice(&encoded); // msgpack payload

    w.write_all(&buf).await.map_err(QfError::Io)?;
    Ok(())
}

/// Decoded frame returned by [`read_frame`].
pub struct RawFrame {
    pub frame_type: u8,
    pub payload: Vec<u8>,
}

/// Read one length-prefixed frame from an async reader.
pub async fn read_frame<R>(r: &mut R) -> Result<RawFrame, QfError>
where
    R: AsyncRead + Unpin,
{
    // Read 4-byte big-endian length prefix.
    let mut header = [0u8; 4];
    r.read_exact(&mut header).await.map_err(QfError::Io)?;
    let frame_len = u32::from_be_bytes(header);

    if frame_len == 0 || frame_len > MAX_FRAME_SIZE {
        return Err(QfError::Codec(format!("invalid frame length: {frame_len}")));
    }

    let mut buf = vec![0u8; frame_len as usize];
    r.read_exact(&mut buf).await.map_err(QfError::Io)?;

    Ok(RawFrame {
        frame_type: buf[0],
        payload: buf[1..].to_vec(),
    })
}

/// Decode a [`Response`] from a raw msgpack payload.
pub fn decode_response(data: &[u8]) -> Result<Response, QfError> {
    rmp_serde::from_slice(data).map_err(|e| QfError::Codec(format!("decode response: {e}")))
}

/// Decode a [`StreamChunk`] from a raw msgpack payload.
pub fn decode_stream_chunk(data: &[u8]) -> Result<StreamChunk, QfError> {
    rmp_serde::from_slice(data).map_err(|e| QfError::Codec(format!("decode stream chunk: {e}")))
}

/// Decode an [`ErrorFrame`] from a raw msgpack payload.
pub fn decode_error(data: &[u8]) -> Result<ErrorFrame, QfError> {
    rmp_serde::from_slice(data).map_err(|e| QfError::Codec(format!("decode error frame: {e}")))
}
