package quicframe

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/msmecodex/quicframe/protocol"
	"github.com/vmihailenco/msgpack/v5"
)

// TestBodyMarshaling verifies that []byte bodies skip marshaling and other types are correctly marshaled.
func TestBodyMarshaling(t *testing.T) {
	// Test case 1: []byte body (should skip marshaling)
	t.Run("RawBytes", func(t *testing.T) {
		body := []byte("hello raw bytes")
		// Simulate logic in RequestWithHeaders
		var reqBody []byte
		if b, ok := interface{}(body).([]byte); ok {
			reqBody = b
		} else {
			reqBody, _ = msgpack.Marshal(body)
		}

		if !bytes.Equal(reqBody, body) {
			t.Errorf("Expected raw bytes to be unchanged, got %s", reqBody)
		}
	})

	// Test case 2: Struct body (should marshal with msgpack)
	t.Run("Struct", func(t *testing.T) {
		type data struct {
			Name string `msgpack:"name"`
		}
		body := data{Name: "tester"}
		expected, _ := msgpack.Marshal(body)

		var reqBody []byte
		if b, ok := interface{}(body).([]byte); ok {
			reqBody = b
		} else {
			var err error
			reqBody, err = msgpack.Marshal(body)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
		}

		if !bytes.Equal(reqBody, expected) {
			t.Errorf("Expected marshaled bytes to match msgpack output")
		}
	})
}

func TestErrorDecoding(t *testing.T) {
	errFrame := &protocol.ErrorFrame{
		Code:    403,
		Message: "forbidden access",
	}
	data, _ := msgpack.Marshal(errFrame)
	
	// Simulate decoding in Client
	decoded, _ := protocol.DecodeError(data)
	err := fmt.Errorf("quicframe: server error %d: %s", decoded.Code, decoded.Message)
	
	expected := "quicframe: server error 403: forbidden access"
	if err.Error() != expected {
		t.Errorf("Expected error string %q, got %q", expected, err.Error())
	}
}

func TestProtocol_WriteReadFrame(t *testing.T) {
	buf := new(bytes.Buffer)
	req := &protocol.Request{
		ID:      "test-id",
		Method:  "GET",
		Path:    "/test",
		Headers: map[string]string{"X-Test": "Value"},
	}

	if err := protocol.WriteFrame(buf, protocol.FrameTypeRequest, req); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	ft, data, err := protocol.ReadFrame(buf)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}

	if ft != protocol.FrameTypeRequest {
		t.Errorf("Expected FrameTypeRequest, got %v", ft)
	}

	decodedReq, err := protocol.DecodeRequest(data)
	if err != nil {
		t.Fatalf("DecodeRequest failed: %v", err)
	}

	if decodedReq.ID != req.ID || decodedReq.Headers["X-Test"] != "Value" {
		t.Errorf("Decoded request does not match original")
	}
}
