//! High-level QuicFrame client — request/response and streaming.
//!
//! ```rust,no_run
//! # use quicframe::{Client, ClientConfig};
//! # #[tokio::main] async fn main() -> anyhow::Result<()> {
//! let client = Client::connect("127.0.0.1:4433", "localhost", ClientConfig::default()).await?;
//!
//! // Unary request
//! let resp = client.get("/ping", Default::default()).await?;
//! println!("status={} body={:?}", resp.status, resp.body);
//!
//! // Streaming response
//! let mut stream = client.stream("GET", "/events", Default::default(), vec![]).await?;
//! while let Some(chunk) = stream.next().await? {
//!     println!("chunk seq={} data={:?}", chunk.seq, chunk.data);
//! }
//! # Ok(()) }
//! ```

use std::{collections::HashMap, sync::Arc, time::Duration};

use serde::Serialize;
use tokio::time::timeout;
use tracing::debug;
use uuid::Uuid;

use crate::{
    error::QfError,
    protocol::{
        self, Request, StreamChunk, FRAME_ERROR, FRAME_PING, FRAME_PONG, FRAME_RESPONSE,
        FRAME_STREAM_DATA, FRAME_STREAM_END,
    },
    transport::{Pool, PoolConfig},
};

// ── Public response types ────────────────────────────────────────────────────

/// A fully-received, non-streaming response.
#[derive(Debug)]
pub struct QfResponse {
    pub status: i32,
    pub headers: HashMap<String, String>,
    /// Raw msgpack-encoded body.  Decode with `rmp_serde::from_slice`.
    pub body: Vec<u8>,
}

impl QfResponse {
    /// Deserialise the body using rmp_serde.
    pub fn decode<T: for<'de> serde::Deserialize<'de>>(&self) -> Result<T, QfError> {
        rmp_serde::from_slice(&self.body).map_err(|e| QfError::Codec(format!("decode body: {e}")))
    }
}

/// A handle for consuming a streaming response.
pub struct StreamHandle {
    recv: quinn::RecvStream,
}

impl StreamHandle {
    fn new(recv: quinn::RecvStream) -> Self {
        Self { recv }
    }

    /// Read the next chunk.  Returns `None` when the server signals `Final`.
    pub async fn next(&mut self) -> Result<Option<StreamChunk>, QfError> {
        let frame = protocol::read_frame(&mut self.recv).await?;
        match frame.frame_type {
            FRAME_STREAM_DATA => {
                let chunk = protocol::decode_stream_chunk(&frame.payload)?;
                if chunk.r#final {
                    return Ok(None);
                }
                Ok(Some(chunk))
            }
            FRAME_STREAM_END => Ok(None),
            FRAME_ERROR => {
                let ef = protocol::decode_error(&frame.payload)?;
                Err(QfError::ServerError {
                    code: ef.code,
                    message: ef.message,
                })
            }
            other => Err(QfError::UnexpectedFrame(other)),
        }
    }

    /// Collect all chunks into a single `Vec<Vec<u8>>`.
    pub async fn collect_all(mut self) -> Result<Vec<Vec<u8>>, QfError> {
        let mut out = Vec::new();
        while let Some(chunk) = self.next().await? {
            out.push(chunk.data);
        }
        Ok(out)
    }
}

// ── Client configuration ─────────────────────────────────────────────────────

/// Configuration for [`Client`].
#[derive(Debug, Clone)]
pub struct ClientConfig {
    /// Pool parameters.
    pub pool: PoolConfig,

    /// Per-request timeout.
    pub request_timeout: Duration,

    /// Number of times to retry a failed request before giving up.
    pub max_retries: u32,

    /// Base backoff interval for retries.
    pub retry_base_delay: Duration,
}

impl Default for ClientConfig {
    fn default() -> Self {
        Self {
            pool: PoolConfig::default(),
            request_timeout: Duration::from_secs(30),
            max_retries: 3,
            retry_base_delay: Duration::from_millis(100),
        }
    }
}

// ── Client ───────────────────────────────────────────────────────────────────

/// QuicFrame client.
///
/// Clone-able: each clone shares the same connection pool.
#[derive(Clone)]
pub struct Client {
    pool: Arc<Pool>,
    cfg: ClientConfig,
    /// Default headers merged into every request.
    default_headers: HashMap<String, String>,
}

impl Client {
    /// Connect to a QuicFrame server.
    ///
    /// `server_name` must match the TLS certificate (SNI).
    pub async fn connect(
        addr: impl ToSocketAddrs,
        server_name: &str,
        cfg: ClientConfig,
    ) -> Result<Self, QfError> {
        let addr = addr
            .to_socket_addrs()
            .map_err(QfError::Io)?
            .next()
            .ok_or_else(|| {
                QfError::Io(std::io::Error::new(
                    std::io::ErrorKind::InvalidInput,
                    "no socket address resolved",
                ))
            })?;

        let pool = Pool::new(addr, server_name, cfg.pool.clone()).await?;
        Ok(Self {
            pool,
            cfg,
            default_headers: HashMap::new(),
        })
    }

    /// Add a default header sent with every request (e.g. `authorization`).
    pub fn with_header(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.default_headers.insert(key.into(), value.into());
        self
    }

    // ── Convenience methods ────────────────────────────────────────────────

    /// Issue a GET request.
    pub async fn get(
        &self,
        path: &str,
        headers: HashMap<String, String>,
    ) -> Result<QfResponse, QfError> {
        self.request("GET", path, headers, vec![]).await
    }

    /// Issue a POST request with a msgpack-encoded body.
    pub async fn post<T: Serialize>(
        &self,
        path: &str,
        body: &T,
        headers: HashMap<String, String>,
    ) -> Result<QfResponse, QfError> {
        let encoded = rmp_serde::to_vec_named(body)
            .map_err(|e| QfError::Codec(format!("encode body: {e}")))?;
        self.request("POST", path, headers, encoded).await
    }

    /// Issue a PUT request with a msgpack-encoded body.
    pub async fn put<T: Serialize>(
        &self,
        path: &str,
        body: &T,
        headers: HashMap<String, String>,
    ) -> Result<QfResponse, QfError> {
        let encoded = rmp_serde::to_vec_named(body)
            .map_err(|e| QfError::Codec(format!("encode body: {e}")))?;
        self.request("PUT", path, headers, encoded).await
    }

    /// Issue a DELETE request.
    pub async fn delete(
        &self,
        path: &str,
        headers: HashMap<String, String>,
    ) -> Result<QfResponse, QfError> {
        self.request("DELETE", path, headers, vec![]).await
    }

    // ── Core request dispatch ──────────────────────────────────────────────

    /// Send a unary (non-streaming) request and wait for the complete response.
    ///
    /// Retries on network-level errors up to `cfg.max_retries` times with
    /// exponential backoff.
    pub async fn request(
        &self,
        method: &str,
        path: &str,
        mut headers: HashMap<String, String>,
        body: Vec<u8>,
    ) -> Result<QfResponse, QfError> {
        // Merge default headers (request-level headers win on conflict).
        for (k, v) in &self.default_headers {
            headers.entry(k.clone()).or_insert_with(|| v.clone());
        }

        let mut last_err = QfError::NoConnections;
        for attempt in 0..=self.cfg.max_retries {
            if attempt > 0 {
                let delay = self.cfg.retry_base_delay * 2u32.pow(attempt - 1);
                tokio::time::sleep(delay).await;
                debug!("retry attempt {attempt} for {method} {path}");
            }

            match self.do_request(method, path, &headers, &body).await {
                Ok(resp) => return Ok(resp),
                Err(e) => {
                    // Only retry on transport errors, not server-side errors.
                    if matches!(e, QfError::ServerError { .. }) {
                        return Err(e);
                    }
                    last_err = e;
                }
            }
        }
        Err(last_err)
    }

    async fn do_request(
        &self,
        method: &str,
        path: &str,
        headers: &HashMap<String, String>,
        body: &[u8],
    ) -> Result<QfResponse, QfError> {
        let conn = self.pool.get_connection().await?;

        // Open a bidirectional stream for this request.
        let (mut send, mut recv) = conn.open_bi().await?;

        let req = Request {
            id: Uuid::new_v4().to_string(),
            method: method.to_string(),
            path: path.to_string(),
            headers: headers.clone(),
            body: body.to_vec(),
            stream: false,
        };

        let result = timeout(self.cfg.request_timeout, async {
            protocol::write_frame(&mut send, protocol::FRAME_REQUEST, &req).await?;
            send.finish()?;

            let frame = protocol::read_frame(&mut recv).await?;
            match frame.frame_type {
                FRAME_RESPONSE => {
                    let resp = protocol::decode_response(&frame.payload)?;
                    Ok(QfResponse {
                        status: resp.status,
                        headers: resp.headers,
                        body: resp.body,
                    })
                }
                FRAME_ERROR => {
                    let ef = protocol::decode_error(&frame.payload)?;
                    Err(QfError::ServerError {
                        code: ef.code,
                        message: ef.message,
                    })
                }
                other => Err(QfError::UnexpectedFrame(other)),
            }
        })
        .await
        .map_err(|_| QfError::Timeout)?;

        result
    }

    // ── Streaming request ──────────────────────────────────────────────────

    /// Open a streaming request.  The returned [`StreamHandle`] yields chunks
    /// as the server sends them.
    ///
    /// The server must respond with `stream: true` in the initial
    /// [`FRAME_RESPONSE`] frame, followed by [`FRAME_STREAM_DATA`] frames.
    pub async fn stream(
        &self,
        method: &str,
        path: &str,
        headers: HashMap<String, String>,
        body: Vec<u8>,
    ) -> Result<(QfResponse, StreamHandle), QfError> {
        let conn = self.pool.get_connection().await?;
        let (mut send, recv) = conn.open_bi().await?;

        let req = Request {
            id: Uuid::new_v4().to_string(),
            method: method.to_string(),
            path: path.to_string(),
            headers,
            body,
            stream: true,
        };

        protocol::write_frame(&mut send, protocol::FRAME_REQUEST, &req).await?;
        send.finish()?;

        // Read the initial response header frame.
        let mut recv = recv;
        let frame = protocol::read_frame(&mut recv).await?;

        let init_resp = match frame.frame_type {
            FRAME_RESPONSE => protocol::decode_response(&frame.payload)?,
            FRAME_ERROR => {
                let ef = protocol::decode_error(&frame.payload)?;
                return Err(QfError::ServerError {
                    code: ef.code,
                    message: ef.message,
                });
            }
            other => return Err(QfError::UnexpectedFrame(other)),
        };

        if !init_resp.stream {
            return Err(QfError::Codec(
                "server responded with stream=false on a streaming request".into(),
            ));
        }

        let meta = QfResponse {
            status: init_resp.status,
            headers: init_resp.headers,
            body: vec![],
        };

        Ok((meta, StreamHandle::new(recv)))
    }

    // ── Keep-alive ping ────────────────────────────────────────────────────

    /// Send a ping frame and wait for the pong.  Returns the round-trip
    /// latency.
    pub async fn ping(&self) -> Result<Duration, QfError> {
        let conn = self.pool.get_connection().await?;
        let (mut send, mut recv) = conn.open_bi().await?;

        let start = std::time::Instant::now();
        let ts = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as i64;

        protocol::write_frame(&mut send, FRAME_PING, &protocol::PingFrame { ts }).await?;
        send.finish()?;

        let frame = protocol::read_frame(&mut recv).await?;
        if frame.frame_type != FRAME_PONG {
            return Err(QfError::UnexpectedFrame(frame.frame_type));
        }

        Ok(start.elapsed())
    }

    /// Gracefully close the connection pool.
    pub async fn close(&self) {
        self.pool.close().await;
    }
}

// ── ToSocketAddrs helper ─────────────────────────────────────────────────────

use std::net::ToSocketAddrs;
