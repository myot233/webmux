package mcphttp

import (
	"bytes"
	"encoding/json"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/jsonutil"
	"github.com/rainchan/webmux/internal/mcpprotocol"
)

const maxInput = 64 << 10

func rpcError(id json.RawMessage, code int, message string) *contract.MCPResponse {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return &contract.MCPResponse{JSONRPC: "2.0", ID: id,
		Error: &contract.MCPError{Code: code, Message: message}}
}

func decode(data []byte) (contract.MCPRequest, *contract.MCPResponse) {
	if !utf8.Valid(data) || !json.Valid(data) {
		return contract.MCPRequest{}, rpcError(nil, -32700, "Parse error")
	}
	var object map[string]json.RawMessage
	if jsonutil.Decode(data, &object) != nil || object == nil {
		return contract.MCPRequest{}, rpcError(nil, -32600, "Invalid request")
	}
	id := object["id"]
	if len(id) > 0 {
		if _, ok := rpcID(id); !ok {
			return contract.MCPRequest{}, rpcError(nil, -32600, "Invalid request")
		}
	}
	for key := range object {
		if key != "jsonrpc" && key != "id" && key != "method" && key != "params" {
			return contract.MCPRequest{}, rpcError(id, -32600, "Invalid request")
		}
	}
	var version, method string
	if json.Unmarshal(object["jsonrpc"], &version) != nil || version != "2.0" ||
		json.Unmarshal(object["method"], &method) != nil || method == "" {
		return contract.MCPRequest{}, rpcError(id, -32600, "Invalid request")
	}
	req, response := mcpprotocol.Decode(data)
	if response != nil {
		response.ID = id
		if len(response.ID) == 0 {
			response.ID = json.RawMessage("null")
		}
	}
	return req, response
}

// rpcID normalizes exact integer JSON numbers without floating point rounding
// or big-integer allocation for maliciously large exponents.
func rpcID(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", false
	}
	if raw[0] == '"' {
		var value string
		if json.Unmarshal(raw, &value) != nil || len(value) == 0 || len(value) > 128 || !utf8.ValidString(value) || !validIDSurrogates(raw) {
			return "", false
		}
		return "s:" + value, true
	}
	if !json.Valid(raw) || (raw[0] != '-' && (raw[0] < '0' || raw[0] > '9')) {
		return "", false
	}
	value := string(raw)
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	exponent := 0
	if at := strings.IndexAny(value, "eE"); at >= 0 {
		exp := value[at+1:]
		value = value[:at]
		minus := strings.HasPrefix(exp, "-")
		exp = strings.TrimLeft(exp, "+-")
		for _, digit := range exp {
			if exponent <= maxInput+32 {
				exponent = exponent*10 + int(digit-'0')
			}
		}
		if minus {
			exponent = -exponent
		}
	}
	if at := strings.IndexByte(value, '.'); at >= 0 {
		exponent -= len(value) - at - 1
		value = value[:at] + value[at+1:]
	}
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return "n:0", true
	}
	trimmed := strings.TrimRight(value, "0")
	exponent += len(value) - len(trimmed)
	value = trimmed
	if exponent < 0 || len(value)+exponent > 16 {
		return "", false
	}
	value += strings.Repeat("0", exponent)
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number > 9007199254740991 {
		return "", false
	}
	if negative {
		value = "-" + value
	}
	return "n:" + value, true
}

func singleHeader(header http.Header, key string) (string, bool) {
	values := header.Values(key)
	if len(values) == 0 {
		return "", true
	}
	if len(values) != 1 || values[0] == "" || strings.TrimSpace(values[0]) != values[0] || strings.Contains(values[0], ",") {
		return "", false
	}
	return values[0], true
}

func sessionHeader(header http.Header) (string, bool) {
	value, ok := singleHeader(header, "Mcp-Session-Id")
	if !ok || value == "" {
		return value, ok
	}
	if len(value) != 43 {
		return "", false
	}
	for _, c := range value {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return "", false
		}
	}
	// A 32-byte unpadded base64url value has two zero padding bits.
	if !strings.ContainsRune("AEIMQUYcgkosw048", rune(value[42])) {
		return "", false
	}
	return value, true
}

func contentTypeOK(header http.Header) bool {
	values := header.Values("Content-Type")
	if len(values) != 1 {
		return false
	}
	typeName, params, err := mime.ParseMediaType(values[0])
	return err == nil && typeName == "application/json" &&
		(params["charset"] == "" || strings.EqualFold(params["charset"], "utf-8"))
}

func acceptOK(header http.Header) bool {
	jsonOK, streamOK := false, false
	for _, value := range header.Values("Accept") {
		for _, item := range strings.Split(value, ",") {
			typeName, params, err := mime.ParseMediaType(strings.TrimSpace(item))
			if err != nil {
				return false
			}
			if quality, exists := params["q"]; exists {
				q, err := strconv.ParseFloat(quality, 64)
				if err != nil || !(q >= 0 && q <= 1) {
					return false
				}
				if q == 0 {
					continue
				}
			}
			jsonOK = jsonOK || typeName == "application/json"
			streamOK = streamOK || typeName == "text/event-stream"
		}
	}
	return jsonOK && streamOK
}

func httpVersion(version string) bool {
	switch version {
	case "", "2025-03-26", "2025-06-18", "2025-11-25":
		return true
	default:
		return false
	}
}

// encoding/json replaces unpaired UTF-16 surrogates with U+FFFD. Reject them
// rather than aliasing distinct wire IDs to the same cancellation key.
func validIDSurrogates(raw []byte) bool {
	for i := 1; i < len(raw)-1; i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if raw[i] != 'u' {
			continue
		}
		code, _ := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		i += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return false
		}
		if code >= 0xd800 && code <= 0xdbff {
			if i+6 >= len(raw)-1 || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, _ := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
