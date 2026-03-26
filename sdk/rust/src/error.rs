use thiserror::Error;

/// All errors that can be returned by the QuicFrame Rust SDK.
#[derive(Debug, Error)]
pub enum QfError {
    /// I/O error on the underlying QUIC stream or connection.
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),

    /// MsgPack encode/decode failure.
    #[error("codec error: {0}")]
    Codec(String),

    /// QUIC connection error (quinn).
    #[error("quic connection error: {0}")]
    Connection(#[from] quinn::ConnectionError),

    /// QUIC write error.
    #[error("quic write error: {0}")]
    Write(#[from] quinn::WriteError),

    /// QUIC read error.
    #[error("quic read error: {0}")]
    Read(#[from] quinn::ReadError),

    /// The server returned a protocol-level error frame.
    #[error("server error {code}: {message}")]
    ServerError { code: i32, message: String },

    /// The connection pool is empty and no new connection could be established.
    #[error("no connections available")]
    NoConnections,

    /// TLS configuration error.
    #[error("tls error: {0}")]
    Tls(String),

    /// A timeout elapsed before the operation completed.
    #[error("timeout")]
    Timeout,

    /// Unexpected frame type received.
    #[error("unexpected frame type 0x{0:02x}")]
    UnexpectedFrame(u8),
}
