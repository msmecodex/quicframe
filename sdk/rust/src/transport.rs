//! Connection pool over QUIC (quinn).
//!
//! The pool maintains up to `max_connections` live [`quinn::Connection`]s to a
//! single server endpoint.  Connections are created on demand and re-used for
//! subsequent requests (QUIC multiplexing means one connection can carry
//! thousands of concurrent streams).
//!
//! Stale / closed connections are detected lazily and replaced.

use std::{net::SocketAddr, sync::Arc, time::Duration};

use quinn::{ClientConfig, Connection, Endpoint};
use rustls::pki_types::ServerName;
use tokio::sync::Mutex;
use tracing::debug;

use crate::error::QfError;

// ── TLS helpers ──────────────────────────────────────────────────────────────

/// Build a [`rustls::ClientConfig`] that **skips** certificate verification.
///
/// ⚠️  For development / testing only.  Do not use in production.
pub fn dangerous_tls_config() -> rustls::ClientConfig {
    #[derive(Debug)]
    struct NoVerifier;

    impl rustls::client::danger::ServerCertVerifier for NoVerifier {
        fn verify_server_cert(
            &self,
            _end_entity: &rustls::pki_types::CertificateDer<'_>,
            _intermediates: &[rustls::pki_types::CertificateDer<'_>],
            _server_name: &ServerName<'_>,
            _ocsp_response: &[u8],
            _now: rustls::pki_types::UnixTime,
        ) -> Result<rustls::client::danger::ServerCertVerified, rustls::Error> {
            Ok(rustls::client::danger::ServerCertVerified::assertion())
        }

        fn verify_tls12_signature(
            &self,
            _message: &[u8],
            _cert: &rustls::pki_types::CertificateDer<'_>,
            _dss: &rustls::DigitallySignedStruct,
        ) -> Result<rustls::client::danger::HandshakeSignatureValid, rustls::Error> {
            Ok(rustls::client::danger::HandshakeSignatureValid::assertion())
        }

        fn verify_tls13_signature(
            &self,
            _message: &[u8],
            _cert: &rustls::pki_types::CertificateDer<'_>,
            _dss: &rustls::DigitallySignedStruct,
        ) -> Result<rustls::client::danger::HandshakeSignatureValid, rustls::Error> {
            Ok(rustls::client::danger::HandshakeSignatureValid::assertion())
        }

        fn supported_verify_schemes(&self) -> Vec<rustls::SignatureScheme> {
            vec![
                rustls::SignatureScheme::ECDSA_NISTP256_SHA256,
                rustls::SignatureScheme::ECDSA_NISTP384_SHA384,
                rustls::SignatureScheme::ED25519,
                rustls::SignatureScheme::RSA_PSS_SHA256,
                rustls::SignatureScheme::RSA_PSS_SHA384,
                rustls::SignatureScheme::RSA_PSS_SHA512,
                rustls::SignatureScheme::RSA_PKCS1_SHA256,
                rustls::SignatureScheme::RSA_PKCS1_SHA384,
                rustls::SignatureScheme::RSA_PKCS1_SHA512,
            ]
        }
    }

    rustls::ClientConfig::builder()
        .dangerous()
        .with_custom_certificate_verifier(Arc::new(NoVerifier))
        .with_no_client_auth()
}

/// Build a production [`rustls::ClientConfig`] that verifies the server
/// certificate against the system root store.
pub fn system_tls_config() -> Result<rustls::ClientConfig, QfError> {
    let roots = rustls::RootCertStore {
        roots: webpki_roots::TLS_SERVER_ROOTS.to_vec(),
    };
    Ok(rustls::ClientConfig::builder()
        .with_root_certificates(roots)
        .with_no_client_auth())
}

// ── Connection pool ──────────────────────────────────────────────────────────

/// Configuration for the connection pool.
#[derive(Debug, Clone)]
pub struct PoolConfig {
    /// Maximum number of live connections to maintain.
    /// New requests wait if all connections are saturated.
    pub max_connections: usize,

    /// ALPN protocol token.  Must match the server's listener.
    pub alpn: Vec<u8>,

    /// Idle timeout passed to QUIC.
    pub idle_timeout: Duration,

    /// Whether to skip TLS certificate verification (development only).
    pub skip_cert_verify: bool,
}

impl Default for PoolConfig {
    fn default() -> Self {
        Self {
            max_connections: 4,
            alpn: b"qf/1".to_vec(),
            idle_timeout: Duration::from_secs(300),
            skip_cert_verify: false,
        }
    }
}

/// A simple connection pool that keeps up to `max_connections` live
/// [`quinn::Connection`]s to a single server endpoint.
pub struct Pool {
    endpoint: Endpoint,
    server_addr: SocketAddr,
    server_name: String,
    cfg: PoolConfig,
    conns: Mutex<Vec<Connection>>,
}

impl Pool {
    /// Create a new pool.  The local QUIC endpoint is bound to `0.0.0.0:0`.
    pub async fn new(
        server_addr: SocketAddr,
        server_name: impl Into<String>,
        cfg: PoolConfig,
    ) -> Result<Arc<Self>, QfError> {
        let tls_cfg = if cfg.skip_cert_verify {
            dangerous_tls_config()
        } else {
            system_tls_config()?
        };

        let mut client_cfg = ClientConfig::new(Arc::new(
            quinn::crypto::rustls::QuicClientConfig::try_from(tls_cfg)
                .map_err(|e| QfError::Tls(e.to_string()))?,
        ));

        let mut transport = quinn::TransportConfig::default();
        transport.max_idle_timeout(Some(
            cfg.idle_timeout
                .try_into()
                .expect("idle timeout out of range"),
        ));
        client_cfg.transport_config(Arc::new(transport));

        let local: SocketAddr = if server_addr.is_ipv6() {
            "[::]:0".parse().unwrap()
        } else {
            "0.0.0.0:0".parse().unwrap()
        };

        let mut endpoint = Endpoint::client(local).map_err(QfError::Io)?;
        endpoint.set_default_client_config(client_cfg);

        Ok(Arc::new(Pool {
            endpoint,
            server_addr,
            server_name: server_name.into(),
            cfg,
            conns: Mutex::new(Vec::new()),
        }))
    }

    /// Acquire a live connection from the pool.
    ///
    /// If all existing connections are closed (e.g. server restart), a new
    /// one is created.  Panics if `max_connections` is 0.
    pub async fn get_connection(&self) -> Result<Connection, QfError> {
        let mut guard = self.conns.lock().await;

        // Evict closed connections.
        guard.retain(|c| c.close_reason().is_none());

        // Re-use an existing live connection if available.
        if let Some(conn) = guard.first().cloned() {
            debug!("pool: re-using existing connection");
            return Ok(conn);
        }

        // Open a new connection.
        debug!("pool: opening new connection to {}", self.server_addr);
        let connecting = self
            .endpoint
            .connect(self.server_addr, &self.server_name)
            .map_err(|e| QfError::Tls(e.to_string()))?;

        let conn = connecting.await?;

        if guard.len() < self.cfg.max_connections {
            guard.push(conn.clone());
        }

        Ok(conn)
    }

    /// Close all pooled connections gracefully.
    pub async fn close(&self) {
        let mut guard = self.conns.lock().await;
        for conn in guard.drain(..) {
            conn.close(0u32.into(), b"pool closed");
        }
        self.endpoint.close(0u32.into(), b"pool closed");
    }
}
