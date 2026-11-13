package api

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/wire"
)

var outputGeneration = regexp.MustCompile(`^[0-9a-f]{16}$`)
var outputCursor = regexp.MustCompile(`^(0|[1-9][0-9]{0,19})$`)

func (s *Server) output(r *http.Request) (contract.TerminalOutput, error) {
	var out contract.TerminalOutput
	q, err := readConversationQuery(r, "generation", "offset", "maxBytes")
	invalid := &badRequest{errors.New("invalid terminal output cursor or page")}
	if err != nil || q.Has("generation") != q.Has("offset") {
		return out, invalid
	}
	var resume *session.ResumePoint
	if q.Has("generation") {
		offset, err := strconv.ParseUint(q.Get("offset"), 10, 64)
		if err != nil || !outputCursor.MatchString(q.Get("offset")) || !outputGeneration.MatchString(q.Get("generation")) {
			return out, invalid
		}
		resume = &session.ResumePoint{Generation: q.Get("generation"), Offset: offset}
	}
	limit := uint64(65536)
	if q.Has("maxBytes") {
		limit, err = strconv.ParseUint(q.Get("maxBytes"), 10, 32)
		if err != nil || !outputCursor.MatchString(q.Get("maxBytes")) || limit < 1 || limit > 65536 {
			return out, invalid
		}
	}
	item, err := s.Manager.Get(r.PathValue("id"))
	if err != nil {
		return out, err
	}
	page, err := item.ReadOutput(resume, int(limit))
	if err != nil {
		return out, err
	}
	return wire.TerminalOutput(page), nil
}
