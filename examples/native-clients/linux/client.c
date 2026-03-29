#include "../../../sdk/rust/include/quicframe.h"

#include <stdio.h>

int main(void) {
    QfClientOptions options = qf_client_options_default();
    options.skip_cert_verify = true;

    QfClientHandle* client = NULL;
    QfErrorInfo error = {0};

    if (!qf_client_connect("127.0.0.1:4433", "localhost", options, &client, &error)) {
        fprintf(stderr, "connect failed (%d): %s\n", error.code, error.message);
        qf_error_free(&error);
        return 1;
    }

    QfResponse response = {0};
    if (!qf_client_request(client, "GET", "/ping", NULL, 0, NULL, 0, &response, &error)) {
        fprintf(stderr, "request failed (%d): %s\n", error.code, error.message);
        qf_error_free(&error);
        qf_client_close(client);
        qf_client_free(client);
        return 1;
    }

    printf("status=%d\n", response.status);
    printf("headers=%s\n", response.headers_json);
    printf("body_len=%zu\n", response.body.len);

    qf_response_free(&response);
    qf_client_close(client);
    qf_client_free(client);
    return 0;
}
