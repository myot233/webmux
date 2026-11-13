// Package mcphttp implements the authenticated-owner, JSON-only MCP HTTP profile.
package mcphttp

import (
	"io"
	"net/http"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !contract.MCPOwner(r.Context()) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="webmux"`)
		h.status(w, http.StatusUnauthorized)
		return
	}
	h.mu.Lock()
	if h.closed || h.ctx.Err() != nil {
		h.mu.Unlock()
		h.status(w, http.StatusServiceUnavailable)
		return
	}
	h.requests.Add(1)
	h.mu.Unlock()
	defer h.requests.Done()
	if r.Method != http.MethodPost && r.Method != http.MethodGet && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "POST, DELETE")
		h.status(w, http.StatusMethodNotAllowed)
		return
	}
	id, ok := sessionHeader(r.Header)
	version, versionOK := singleHeader(r.Header, "Mcp-Protocol-Version")
	if !ok || !versionOK || !httpVersion(version) {
		h.status(w, http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodPost {
		h.nonPost(w, r, id, version)
		return
	}
	if !contentTypeOK(r.Header) {
		h.status(w, http.StatusUnsupportedMediaType)
		return
	}
	if !acceptOK(r.Header) {
		h.status(w, http.StatusNotAcceptable)
		return
	}
	data, status := readBody(w, r)
	if status != 0 {
		h.status(w, status)
		return
	}
	req, response := decode(data)
	if response != nil {
		h.respond(w, http.StatusBadRequest, response)
		return
	}
	if !validParams(req) {
		h.respond(w, http.StatusBadRequest, rpcError(req.ID, -32602, "Invalid parameters"))
		return
	}
	if req.Method == "initialize" {
		if id != "" || len(req.ID) == 0 {
			h.respond(w, http.StatusBadRequest, rpcError(req.ID, -32600, "Invalid initialize request"))
			return
		}
		h.initialize(w, req)
		return
	}
	if id == "" {
		h.status(w, http.StatusBadRequest)
		return
	}
	e, status := h.lookup(id, version)
	if status != 0 {
		h.status(w, status)
		return
	}
	if len(req.ID) == 0 {
		h.notification(w, e, req)
		return
	}
	if req.Method == "notifications/initialized" || req.Method == "notifications/cancelled" {
		h.respond(w, http.StatusBadRequest, rpcError(req.ID, -32600, "Invalid request"))
		return
	}
	h.dispatch(w, e, req)
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, int) {
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
	if r.ContentLength > maxInput {
		return nil, http.StatusRequestEntityTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxInput+1))
	if len(data) > maxInput {
		return nil, http.StatusRequestEntityTooLarge
	}
	if err != nil {
		return nil, http.StatusBadRequest
	}
	return data, 0
}

func (h *Handler) nonPost(w http.ResponseWriter, r *http.Request, id, version string) {
	if id == "" {
		h.status(w, http.StatusBadRequest)
		return
	}
	e, status := h.lookup(id, version)
	if status != 0 {
		h.status(w, status)
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Allow", "POST, DELETE")
		h.status(w, http.StatusMethodNotAllowed)
		return
	}
	if _, status := readBody(w, r); status != 0 {
		h.status(w, status)
		return
	}
	h.mu.Lock()
	if h.sessions[e.id] != e {
		h.mu.Unlock()
		h.status(w, http.StatusNotFound)
		return
	}
	cancels := h.removeLocked(e)
	h.mu.Unlock()
	cancelAll(cancels)
	e.wg.Wait()
	h.status(w, http.StatusNoContent)
}

func (h *Handler) initialize(w http.ResponseWriter, req contract.MCPRequest) {
	e, status := h.allocate()
	if status != 0 {
		h.status(w, status)
		return
	}
	keep := false
	defer func() {
		if !keep {
			h.mu.Lock()
			cancels := h.removeLocked(e)
			h.mu.Unlock()
			cancelAll(cancels)
		}
	}()
	response, status, call := h.execute(e, req)
	if call != nil {
		defer h.release(e, call)
	}
	if response == nil {
		h.status(w, status)
		return
	}
	data, bounded := encodeOrError(response)
	success := response.Error == nil && bounded
	if success {
		w.Header().Set("Mcp-Session-Id", e.id)
	}
	keep = h.writeJSON(w, status, data) == nil && success
}

func (h *Handler) dispatch(w http.ResponseWriter, e *entry, req contract.MCPRequest) {
	response, status, call := h.execute(e, req)
	if call != nil {
		defer h.release(e, call)
	}
	if response == nil {
		h.status(w, status)
		return
	}
	h.respond(w, status, response)
}

func (h *Handler) execute(e *entry, req contract.MCPRequest) (*contract.MCPResponse, int, *inflight) {
	key, _ := rpcID(req.ID)
	ctx, call, status := h.begin(e, key, req.Method == "tools/call")
	if status != 0 {
		return nil, status, nil
	}
	response, err := e.protocol.Dispatch(ctx, req)
	cancelled := h.finish(e, call)
	if cancelled {
		response = rpcError(req.ID, -32000, "Request cancelled")
		response.Error.Data = []byte(`{"kind":"request_cancelled"}`)
	} else if err != nil || response == nil {
		response = rpcError(req.ID, -32603, "Internal error")
	}
	status = http.StatusOK
	if response.Error != nil && (response.Error.Code == -32600 || response.Error.Code == -32602 || response.Error.Code == -32700) {
		status = http.StatusBadRequest
	}
	return response, status, call
}

func (h *Handler) notification(w http.ResponseWriter, e *entry, req contract.MCPRequest) {
	if req.Method == "notifications/cancelled" {
		key, _ := cancellation(req.Params)
		if !h.cancelRequest(e, key) {
			h.status(w, http.StatusNotFound)
			return
		}
		h.status(w, http.StatusAccepted)
		return
	}
	h.mu.Lock()
	if h.closed || h.ctx.Err() != nil || h.sessions[e.id] != e || h.expired(e, h.now()) {
		h.mu.Unlock()
		h.status(w, http.StatusNotFound)
		return
	}
	h.wg.Add(1)
	e.wg.Add(1)
	h.mu.Unlock()
	// Only the shared protocol may change initialization state. All other
	// notifications are ignored; a notification can never execute a tool.
	var response *contract.MCPResponse
	if req.Method == "notifications/initialized" {
		response, _ = e.protocol.Dispatch(h.ctx, req)
	}
	h.mu.Lock()
	e.lastActivity = h.now()
	h.mu.Unlock()
	e.wg.Done()
	h.wg.Done()
	if response != nil && response.Error != nil {
		h.respond(w, http.StatusBadRequest, response)
		return
	}
	h.status(w, http.StatusAccepted)
}

var _ http.Handler = (*Handler)(nil)
