using System;
using System.Runtime.InteropServices;

internal static class QuicFrameNative
{
    [StructLayout(LayoutKind.Sequential)]
    internal struct QfClientOptions
    {
        public UIntPtr max_connections;
        public ulong idle_timeout_ms;
        public ulong request_timeout_ms;
        public uint max_retries;
        public ulong retry_base_delay_ms;
        [MarshalAs(UnmanagedType.I1)]
        public bool skip_cert_verify;
    }

    [DllImport("quicframe", CallingConvention = CallingConvention.Cdecl)]
    internal static extern QfClientOptions qf_client_options_default();
}

/*
Windows packaging:
- build `quicframe.dll` from the Rust crate
- ship `quicframe.lib` import library with the app
- map the remaining structs/functions from `quicframe.h` using `DllImport`
*/
