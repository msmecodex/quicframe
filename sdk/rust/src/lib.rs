//! # quicframe
//!
//! Production-grade Rust client SDK for the QuicFrame QUIC-native API framework.
//!
//! QuicFrame replaces HTTP with raw QUIC streams and MsgPack framing for
//! ultra-low-latency, multiplexed, bidirectional communication.
//!
//! ## Quickstart
//!
//! ```rust,no_run
//! use quicframe::{Client, ClientConfig};
//! use std::collections::HashMap;
//!
//! #[tokio::main]
//! async fn main() -> Result<(), Box<dyn std::error::Error>> {
//!     let mut cfg = ClientConfig::default();
//!     cfg.pool.skip_cert_verify = true; // dev only
//!
//!     let client = Client::connect("127.0.0.1:4433", "localhost", cfg).await?;
//!
//!     // Simple GET
//!     let resp = client.get("/ping", HashMap::new()).await?;
//!     println!("status={}", resp.status);
//!
//!     // POST with msgpack body
//!     let resp = client.post("/api/v1/users", &serde_json::json!({"name":"Alice"}), HashMap::new()).await?;
//!
//!     // Streaming GET
//!     let (meta, mut stream) = client.stream("GET", "/events", HashMap::new(), vec![]).await?;
//!     while let Some(chunk) = stream.next().await? {
//!         println!("chunk: {:?}", chunk.data);
//!     }
//!
//!     client.close().await;
//!     Ok(())
//! }
//! ```

pub mod client;
pub mod error;
pub mod ffi;
pub mod protocol;
pub mod transport;

// Re-export the most commonly used items at the crate root.
pub use client::{Client, ClientConfig, QfResponse, StreamHandle};
pub use error::QfError;
pub use transport::PoolConfig;
