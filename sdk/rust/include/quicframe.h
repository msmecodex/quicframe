#ifndef QUICFRAME_H
#define QUICFRAME_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct QfClientHandle QfClientHandle;

typedef struct {
    size_t max_connections;
    uint64_t idle_timeout_ms;
    uint64_t request_timeout_ms;
    uint32_t max_retries;
    uint64_t retry_base_delay_ms;
    bool skip_cert_verify;
} QfClientOptions;

typedef struct {
    const char* key;
    const char* value;
} QfHeader;

typedef struct {
    uint8_t* ptr;
    size_t len;
} QfByteBuffer;

typedef struct {
    int32_t status;
    char* headers_json;
    QfByteBuffer body;
} QfResponse;

typedef struct {
    int32_t code;
    char* message;
} QfErrorInfo;

QfClientOptions qf_client_options_default(void);

bool qf_client_connect(
    const char* addr,
    const char* server_name,
    QfClientOptions options,
    QfClientHandle** out_client,
    QfErrorInfo* out_error
);

bool qf_client_request(
    QfClientHandle* client,
    const char* method,
    const char* path,
    const QfHeader* headers,
    size_t headers_len,
    const uint8_t* body_ptr,
    size_t body_len,
    QfResponse* out_response,
    QfErrorInfo* out_error
);

bool qf_client_ping(
    QfClientHandle* client,
    uint64_t* out_latency_ms,
    QfErrorInfo* out_error
);

void qf_client_close(QfClientHandle* client);
void qf_client_free(QfClientHandle* client);
void qf_response_free(QfResponse* response);
void qf_error_free(QfErrorInfo* error);
void qf_buffer_free(QfByteBuffer* buffer);

#ifdef __cplusplus
}
#endif

#endif
