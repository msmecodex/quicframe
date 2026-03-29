import Foundation

/*
Import `quicframe.h` in a bridging header or module map, then wrap the C ABI.

Example usage:

var options = qf_client_options_default()
options.skip_cert_verify = true

var client: UnsafeMutablePointer<QfClientHandle>?
var error = QfErrorInfo()

let ok = "127.0.0.1:4433".withCString { addr in
    "localhost".withCString { serverName in
        qf_client_connect(addr, serverName, options, &client, &error)
    }
}

if !ok, let message = error.message {
    print("connect failed: \(String(cString: message))")
    qf_error_free(&error)
}

Build outputs:
- macOS: `libquicframe.dylib`
- iOS device/simulator: prefer `staticlib` slices combined as `.xcframework`
*/
