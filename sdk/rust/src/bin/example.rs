//! Standalone example that exercises the QuicFrame Rust SDK against a running server.
//!
//! Run with:
//!   cargo run --bin qf-example -- --addr 127.0.0.1:4433

use std::collections::HashMap;

use quicframe::{Client, ClientConfig, PoolConfig};
use tracing::info;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt()
        .with_env_filter("quicframe=debug,qf_example=info")
        .init();

    let addr = std::env::args()
        .skip_while(|a| a != "--addr")
        .nth(1)
        .unwrap_or_else(|| "127.0.0.1:4433".into());

    info!("connecting to {addr}");

    let mut cfg = ClientConfig::default();
    cfg.pool = PoolConfig {
        max_connections: 4,
        alpn: b"qf/1".to_vec(),
        skip_cert_verify: true, // dev self-signed cert
        ..Default::default()
    };

    let mut headers = HashMap::new();
    // For protected endpoints add: headers.insert("authorization".into(), "Bearer <token>".into());

    let client = Client::connect(&addr as &str, "localhost", cfg).await?;

    // ── Ping ─────────────────────────────────────────────────────────────────
    let rtt = client.ping().await?;
    info!("ping RTT = {:?}", rtt);

    // ── GET /ping ─────────────────────────────────────────────────────────────
    let resp = client.get("/ping", HashMap::new()).await?;
    info!("GET /ping  → status={}", resp.status);
    let body: HashMap<String, String> = resp.decode()?;
    info!("  body = {body:?}");

    // ── GET /info ─────────────────────────────────────────────────────────────
    let resp = client.get("/info", HashMap::new()).await?;
    info!("GET /info  → status={}", resp.status);

    // ── POST /api/v1/users ───────────────────────────────────────────────────
    // Add JWT header for protected routes.
    headers.insert(
        "authorization".into(),
        "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.DEMO".into(),
    );
    let user = std::collections::HashMap::from([("name", "Alice"), ("email", "alice@example.com")]);
    let resp = client.post("/api/v1/users", &user, headers.clone()).await?;
    info!("POST /api/v1/users → status={}", resp.status);

    // ── GET /stream/5 ─────────────────────────────────────────────────────────
    let (meta, mut stream) = client
        .stream("GET", "/stream/5", HashMap::new(), vec![])
        .await?;
    info!("GET /stream/5 → status={}", meta.status);
    let mut seq = 0u32;
    while let Some(chunk) = stream.next().await? {
        info!("  chunk seq={} len={}", seq, chunk.data.len());
        seq += 1;
    }
    info!("stream finished after {seq} chunks");

    client.close().await;
    info!("done");
    Ok(())
}
