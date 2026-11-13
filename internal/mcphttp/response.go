package mcphttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

const maxOutput = (8 << 20) + (64 << 10)

// encodeResponse reuses the protocol's already encoded result. Encoding the
// entire response with json.Encoder would first allocate an unbounded second
// copy, even when its destination is a size-limited writer.
func encodeResponse(response *contract.MCPResponse) ([]byte, bool) {
	id := response.ID
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	if len(id) > maxInput || !json.Valid(id) {
		return nil, false
	}
	parts := [][]byte{[]byte(`{"jsonrpc":"2.0","id":`), id}
	if response.Error == nil {
		if len(response.Result) == 0 || len(response.Result) > maxOutput || !json.Valid(response.Result) {
			return nil, false
		}
		parts = append(parts, []byte(`,"result":`), response.Result, []byte("}\n"))
	} else {
		err := response.Error
		// Protocol error messages are controlled constants, never tool output.
		if len(err.Message) > 1024 {
			return nil, false
		}
		message, marshalErr := json.Marshal(err.Message)
		if marshalErr != nil {
			return nil, false
		}
		parts = append(parts, []byte(`,"error":{"code":`), []byte(strconv.Itoa(err.Code)), []byte(`,"message":`), message)
		if len(err.Data) > 0 {
			if len(err.Data) > maxOutput || !json.Valid(err.Data) {
				return nil, false
			}
			parts = append(parts, []byte(`,"data":`), err.Data)
		}
		parts = append(parts, []byte("}}\n"))
	}
	size := 0
	for _, part := range parts {
		if len(part) > maxOutput-size {
			return nil, false
		}
		size += len(part)
	}
	data := make([]byte, 0, size)
	for _, part := range parts {
		data = append(data, part...)
	}
	return data, true
}

func encodeOrError(response *contract.MCPResponse) ([]byte, bool) {
	data, ok := encodeResponse(response)
	if !ok {
		// Fail before writing any header or byte. Never truncate tool fields.
		data, _ = encodeResponse(rpcError(response.ID, -32603, "Response exceeds encoding limit"))
	}
	return data, ok
}

func (h *Handler) respond(w http.ResponseWriter, status int, response *contract.MCPResponse) {
	data, _ := encodeOrError(response)
	_ = h.writeJSON(w, status, data)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data []byte) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
	w.WriteHeader(status)
	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	// net/http may buffer a small response after Write returns. Observe flush
	// failure before retaining an initialized session or releasing call slots.
	err = controller.Flush()
	if errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}

func (h *Handler) status(w http.ResponseWriter, status int) {
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "1")
	}
	w.Header().Set("Content-Length", "0")
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
	w.WriteHeader(status)
}
