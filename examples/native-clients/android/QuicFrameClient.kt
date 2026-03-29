package com.quicframe.demo

object QuicFrameNative {
    init {
        System.loadLibrary("quicframe")
    }

    external fun qf_client_options_default(): QfClientOptions
}

data class QfClientOptions(
    val max_connections: Long,
    val idle_timeout_ms: Long,
    val request_timeout_ms: Long,
    val max_retries: Int,
    val retry_base_delay_ms: Long,
    val skip_cert_verify: Boolean,
)

/*
JNI layer suggestion:
- expose thin Kotlin-friendly functions that internally call the C ABI from `quicframe.h`
- package `libquicframe.so` under `src/main/jniLibs/<abi>/`
- supported Android Rust targets:
  aarch64-linux-android
  armv7-linux-androideabi
  x86_64-linux-android
*/
