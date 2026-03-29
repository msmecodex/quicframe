# Native Clients via C Bindings

These examples consume the Rust SDK through the C ABI exposed in `sdk/rust/include/quicframe.h`.

Targets covered:

- Android via JNI + `libquicframe.so`
- iOS via Swift/Objective-C bridge + static slices / xcframework
- Linux via `libquicframe.so`
- macOS via `libquicframe.dylib`
- Windows via `quicframe.dll`

The exported ABI is intentionally small:

- `qf_client_connect`
- `qf_client_request`
- `qf_client_ping`
- `qf_client_close`
- memory cleanup helpers

See `docs/ffi-sdk.md` for build and packaging instructions.
