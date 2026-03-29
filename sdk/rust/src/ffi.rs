use std::{
    collections::HashMap,
    ffi::{c_char, CStr, CString},
    ptr, slice,
    sync::OnceLock,
    time::Duration,
};

use crate::{Client, ClientConfig, QfError};

fn runtime() -> &'static tokio::runtime::Runtime {
    static RUNTIME: OnceLock<tokio::runtime::Runtime> = OnceLock::new();
    RUNTIME.get_or_init(|| {
        tokio::runtime::Builder::new_multi_thread()
            .enable_all()
            .build()
            .expect("failed to create tokio runtime for quicframe ffi")
    })
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct QfClientOptions {
    pub max_connections: usize,
    pub idle_timeout_ms: u64,
    pub request_timeout_ms: u64,
    pub max_retries: u32,
    pub retry_base_delay_ms: u64,
    pub skip_cert_verify: bool,
}

impl Default for QfClientOptions {
    fn default() -> Self {
        let cfg = ClientConfig::default();
        Self {
            max_connections: cfg.pool.max_connections,
            idle_timeout_ms: cfg.pool.idle_timeout.as_millis() as u64,
            request_timeout_ms: cfg.request_timeout.as_millis() as u64,
            max_retries: cfg.max_retries,
            retry_base_delay_ms: cfg.retry_base_delay.as_millis() as u64,
            skip_cert_verify: cfg.pool.skip_cert_verify,
        }
    }
}

impl From<QfClientOptions> for ClientConfig {
    fn from(value: QfClientOptions) -> Self {
        let mut cfg = ClientConfig::default();
        cfg.pool.max_connections = value.max_connections.max(1);
        cfg.pool.idle_timeout = Duration::from_millis(value.idle_timeout_ms.max(1));
        cfg.pool.skip_cert_verify = value.skip_cert_verify;
        cfg.request_timeout = Duration::from_millis(value.request_timeout_ms.max(1));
        cfg.max_retries = value.max_retries;
        cfg.retry_base_delay = Duration::from_millis(value.retry_base_delay_ms.max(1));
        cfg
    }
}

#[repr(C)]
pub struct QfHeader {
    pub key: *const c_char,
    pub value: *const c_char,
}

#[repr(C)]
#[derive(Default)]
pub struct QfByteBuffer {
    pub ptr: *mut u8,
    pub len: usize,
}

impl QfByteBuffer {
    fn from_vec(mut bytes: Vec<u8>) -> Self {
        let buffer = Self {
            ptr: bytes.as_mut_ptr(),
            len: bytes.len(),
        };
        std::mem::forget(bytes);
        buffer
    }
}

#[repr(C)]
#[derive(Default)]
pub struct QfResponse {
    pub status: i32,
    pub headers_json: *mut c_char,
    pub body: QfByteBuffer,
}

#[repr(C)]
#[derive(Default)]
pub struct QfErrorInfo {
    pub code: i32,
    pub message: *mut c_char,
}

pub struct QfClientHandle {
    client: Client,
}

unsafe fn read_cstr<'a>(ptr: *const c_char, field: &str) -> Result<&'a str, QfError> {
    if ptr.is_null() {
        return Err(QfError::Codec(format!("{field} cannot be null")));
    }

    CStr::from_ptr(ptr)
        .to_str()
        .map_err(|_| QfError::Codec(format!("{field} must be valid UTF-8")))
}

unsafe fn read_headers(
    headers: *const QfHeader,
    headers_len: usize,
) -> Result<HashMap<String, String>, QfError> {
    if headers.is_null() || headers_len == 0 {
        return Ok(HashMap::new());
    }

    let mut map = HashMap::with_capacity(headers_len);
    for header in slice::from_raw_parts(headers, headers_len) {
        let key = read_cstr(header.key, "header key")?;
        let value = read_cstr(header.value, "header value")?;
        map.insert(key.to_owned(), value.to_owned());
    }
    Ok(map)
}

fn into_c_string(value: String) -> *mut c_char {
    CString::new(value)
        .unwrap_or_else(|_| CString::new("string contained interior NUL").expect("static string"))
        .into_raw()
}

fn set_error(out_error: *mut QfErrorInfo, err: QfError) {
    if out_error.is_null() {
        return;
    }

    unsafe {
        ptr::write(
            out_error,
            QfErrorInfo {
                code: error_code(&err),
                message: into_c_string(err.to_string()),
            },
        );
    }
}

fn set_response(
    out_response: *mut QfResponse,
    status: i32,
    headers: HashMap<String, String>,
    body: Vec<u8>,
) -> Result<(), QfError> {
    if out_response.is_null() {
        return Err(QfError::Codec("out_response cannot be null".into()));
    }

    let headers_json = serde_json::to_string(&headers)
        .map_err(|e| QfError::Codec(format!("encode headers json: {e}")))?;

    unsafe {
        ptr::write(
            out_response,
            QfResponse {
                status,
                headers_json: into_c_string(headers_json),
                body: QfByteBuffer::from_vec(body),
            },
        );
    }

    Ok(())
}

fn error_code(err: &QfError) -> i32 {
    match err {
        QfError::Io(_) => 1,
        QfError::Codec(_) => 2,
        QfError::Connection(_) => 3,
        QfError::Write(_) => 4,
        QfError::ClosedStream(_) => 5,
        QfError::Read(_) => 6,
        QfError::ServerError { .. } => 7,
        QfError::NoConnections => 8,
        QfError::Tls(_) => 9,
        QfError::Timeout => 10,
        QfError::UnexpectedFrame(_) => 11,
    }
}

#[no_mangle]
pub extern "C" fn qf_client_options_default() -> QfClientOptions {
    QfClientOptions::default()
}

#[no_mangle]
pub unsafe extern "C" fn qf_client_connect(
    addr: *const c_char,
    server_name: *const c_char,
    options: QfClientOptions,
    out_client: *mut *mut QfClientHandle,
    out_error: *mut QfErrorInfo,
) -> bool {
    if out_client.is_null() {
        set_error(
            out_error,
            QfError::Codec("out_client cannot be null".into()),
        );
        return false;
    }

    let addr = match read_cstr(addr, "addr") {
        Ok(value) => value,
        Err(err) => {
            set_error(out_error, err);
            return false;
        }
    };

    let server_name = match read_cstr(server_name, "server_name") {
        Ok(value) => value,
        Err(err) => {
            set_error(out_error, err);
            return false;
        }
    };

    match runtime().block_on(Client::connect(addr, server_name, options.into())) {
        Ok(client) => {
            ptr::write(
                out_client,
                Box::into_raw(Box::new(QfClientHandle { client })),
            );
            true
        }
        Err(err) => {
            set_error(out_error, err);
            false
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn qf_client_request(
    client: *mut QfClientHandle,
    method: *const c_char,
    path: *const c_char,
    headers: *const QfHeader,
    headers_len: usize,
    body_ptr: *const u8,
    body_len: usize,
    out_response: *mut QfResponse,
    out_error: *mut QfErrorInfo,
) -> bool {
    if client.is_null() {
        set_error(out_error, QfError::Codec("client cannot be null".into()));
        return false;
    }

    let method = match read_cstr(method, "method") {
        Ok(value) => value,
        Err(err) => {
            set_error(out_error, err);
            return false;
        }
    };

    let path = match read_cstr(path, "path") {
        Ok(value) => value,
        Err(err) => {
            set_error(out_error, err);
            return false;
        }
    };

    let headers = match read_headers(headers, headers_len) {
        Ok(value) => value,
        Err(err) => {
            set_error(out_error, err);
            return false;
        }
    };

    let body = if body_ptr.is_null() || body_len == 0 {
        Vec::new()
    } else {
        slice::from_raw_parts(body_ptr, body_len).to_vec()
    };

    let handle = &(*client).client;
    match runtime().block_on(handle.request(method, path, headers, body)) {
        Ok(response) => match set_response(
            out_response,
            response.status,
            response.headers,
            response.body,
        ) {
            Ok(()) => true,
            Err(err) => {
                set_error(out_error, err);
                false
            }
        },
        Err(err) => {
            set_error(out_error, err);
            false
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn qf_client_ping(
    client: *mut QfClientHandle,
    out_latency_ms: *mut u64,
    out_error: *mut QfErrorInfo,
) -> bool {
    if client.is_null() {
        set_error(out_error, QfError::Codec("client cannot be null".into()));
        return false;
    }

    if out_latency_ms.is_null() {
        set_error(
            out_error,
            QfError::Codec("out_latency_ms cannot be null".into()),
        );
        return false;
    }

    let handle = &(*client).client;
    match runtime().block_on(handle.ping()) {
        Ok(latency) => {
            ptr::write(out_latency_ms, latency.as_millis() as u64);
            true
        }
        Err(err) => {
            set_error(out_error, err);
            false
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn qf_client_close(client: *mut QfClientHandle) {
    if client.is_null() {
        return;
    }

    runtime().block_on((*client).client.close());
}

#[no_mangle]
pub unsafe extern "C" fn qf_client_free(client: *mut QfClientHandle) {
    if client.is_null() {
        return;
    }

    drop(Box::from_raw(client));
}

#[no_mangle]
pub unsafe extern "C" fn qf_response_free(response: *mut QfResponse) {
    if response.is_null() {
        return;
    }

    if !(*response).headers_json.is_null() {
        let _ = CString::from_raw((*response).headers_json);
        (*response).headers_json = ptr::null_mut();
    }

    qf_buffer_free(&mut (*response).body);
    (*response).status = 0;
}

#[no_mangle]
pub unsafe extern "C" fn qf_error_free(error: *mut QfErrorInfo) {
    if error.is_null() {
        return;
    }

    if !(*error).message.is_null() {
        let _ = CString::from_raw((*error).message);
        (*error).message = ptr::null_mut();
    }

    (*error).code = 0;
}

#[no_mangle]
pub unsafe extern "C" fn qf_buffer_free(buffer: *mut QfByteBuffer) {
    if buffer.is_null() {
        return;
    }

    if !(*buffer).ptr.is_null() && (*buffer).len > 0 {
        let _ = Vec::from_raw_parts((*buffer).ptr, (*buffer).len, (*buffer).len);
    }

    (*buffer).ptr = ptr::null_mut();
    (*buffer).len = 0;
}
