# Rust FFI SDK

The Rust SDK now exports a C ABI so the same client core can be reused from Android, iOS, Linux, macOS, and Windows.

Files:

- Rust crate: `sdk/rust`
- C header: `sdk/rust/include/quicframe.h`
- FFI implementation: `sdk/rust/src/ffi.rs`
- platform examples: `examples/native-clients`

## Exported ABI

The crate builds as:

- `cdylib` for dynamic linking
- `staticlib` for Apple packaging and other static-link scenarios
- `rlib` for native Rust consumers

Primary exported functions:

- `qf_client_options_default`
- `qf_client_connect`
- `qf_client_request`
- `qf_client_ping`
- `qf_client_close`
- `qf_client_free`
- `qf_response_free`
- `qf_error_free`
- `qf_buffer_free`

`qf_client_request` accepts raw bytes for the body and returns:

- status code
- headers as JSON text
- raw response bytes

That keeps the ABI stable while letting each platform decode MsgPack in its own language layer.

## Build

### Linux

```bash
cd sdk/rust
cargo build --release --target x86_64-unknown-linux-gnu
```

Output:

- `target/x86_64-unknown-linux-gnu/release/libquicframe.so`

### Android

```bash
cd sdk/rust
cargo build --release --target aarch64-linux-android
cargo build --release --target armv7-linux-androideabi
cargo build --release --target x86_64-linux-android
```

Ship each generated `libquicframe.so` inside the Android app's `jniLibs/<abi>/`.

### macOS

```bash
cd sdk/rust
cargo build --release --target aarch64-apple-darwin
cargo build --release --target x86_64-apple-darwin
```

Outputs:

- `libquicframe.dylib`
- `libquicframe.a`

### iOS

For iOS, the C ABI is the same, but packaging is usually done as static libraries combined into an `.xcframework`.

```bash
cd sdk/rust
cargo build --release --target aarch64-apple-ios
cargo build --release --target aarch64-apple-ios-sim
cargo build --release --target x86_64-apple-ios
```

Then package the generated `libquicframe.a` files into an xcframework together with `sdk/rust/include/quicframe.h`.

### Windows

```bash
cd sdk/rust
cargo build --release --target x86_64-pc-windows-msvc
```

Outputs:

- `quicframe.dll`
- `quicframe.lib`

## Ownership Rules

Memory allocated by Rust must be freed with the exported helpers:

- call `qf_response_free` after reading a response
- call `qf_error_free` after reading an error
- call `qf_client_close` before `qf_client_free`

## Platform Notes

### Android

Use JNI to wrap the C ABI into a Kotlin-friendly API. A sample stub is in `examples/native-clients/android/QuicFrameClient.kt`.

### Apple platforms

Use the header from Swift/Objective-C. A sample wrapper sketch is in `examples/native-clients/apple/QuicFrameClient.swift`.

### Linux

A tiny C example is in `examples/native-clients/linux/client.c`.

### Windows

Use `DllImport` or C/C++ against the same header. A C# interop stub is in `examples/native-clients/windows/QuicFrameClient.cs`.
