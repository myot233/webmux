package mcpcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/rainchan/webmux/internal/agentguide"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/mcpprotocol"
)

var generation = regexp.MustCompile(`^[0-9a-f]{16}$`)
var decimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,19})$`)

func unsigned(s string) (uint64, error) {
	if !decimal.MatchString(s) {
		return 0, errors.New("invalid cursor")
	}
	return strconv.ParseUint(s, 10, 64)
}
func (c Client) readTools() []mcpprotocol.Tool {
	return []mcpprotocol.Tool{tool("session_read", "Read committed exportable chat activity, or a bounded raw terminal replay page encoded as base64. Cursors are decimal strings. Reports truncation, resets and missing history; never acquires terminal control.", true, map[string]any{
		"sessionId": text(), "view": enumeration("activity", "terminal"),
		"afterPosition": text(), "itemId": text(), "textOffset": text(), "limit": integer(1, 100), "maxBytesPerItem": integer(4, 65536),
		"generation": text(), "offset": text(), "maxBytes": integer(1, 65536),
	}, []string{"sessionId"}, c.read)}
}

type readArguments struct {
	SessionID       string `json:"sessionId"`
	View            string `json:"view,omitempty"`
	AfterPosition   string `json:"afterPosition,omitempty"`
	ItemID          string `json:"itemId,omitempty"`
	TextOffset      string `json:"textOffset,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	MaxBytesPerItem int    `json:"maxBytesPerItem,omitempty"`
	Generation      string `json:"generation,omitempty"`
	Offset          string `json:"offset,omitempty"`
	MaxBytes        int    `json:"maxBytes,omitempty"`
}

func (c Client) read(ctx context.Context, raw json.RawMessage) (any, error) {
	a := readArguments{View: "activity", AfterPosition: "0", TextOffset: "0", Limit: 20, MaxBytesPerItem: 4000, MaxBytes: 65536}
	if decode(raw, &a) != nil || !identity.MatchString(a.SessionID) {
		return nil, errors.New("invalid session_read arguments")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	switch a.View {
	case "activity":
		if a.Generation != "" || a.Offset != "" || a.MaxBytes != 65536 {
			return nil, errors.New("terminal cursors require terminal view")
		}
		return c.readActivity(ctx, a)
	case "terminal":
		if a.AfterPosition != "0" || a.TextOffset != "0" || a.ItemID != "" || a.Limit != 20 || a.MaxBytesPerItem != 4000 {
			return nil, errors.New("activity cursors require activity view")
		}
		return c.readTerminal(ctx, a)
	default:
		return nil, errors.New("view must be activity or terminal")
	}
}
func (c Client) readActivity(ctx context.Context, a readArguments) (any, error) {
	after, e1 := unsigned(a.AfterPosition)
	offset, e2 := unsigned(a.TextOffset)
	if e1 != nil || e2 != nil || after > 1<<63-1 || offset > 1<<63-1 || a.Limit < 1 || a.Limit > 100 || a.MaxBytesPerItem < 4 || a.MaxBytesPerItem > 65536 || a.Limit*a.MaxBytesPerItem > 256<<10 || a.ItemID == "" && offset != 0 || a.ItemID != "" && !regexpHistoryID.MatchString(a.ItemID) {
		return nil, errors.New("invalid activity cursor or page")
	}
	var session contract.SessionInfo
	if err := c.API.GetBounded(ctx, "/api/sessions/"+url.PathEscape(a.SessionID), responseLimit, &session); err != nil {
		return nil, apiError(err)
	}
	if !regexpHistoryID.MatchString(session.ConversationID) {
		return nil, errors.New("session has no durable conversation history")
	}
	path := "/api/conversations/" + session.ConversationID + "/activities"
	if a.ItemID != "" {
		q := url.Values{"textOffset": {a.TextOffset}, "maxBytes": {strconv.Itoa(a.MaxBytesPerItem)}}
		var item contract.ConversationActivity
		if err := c.API.GetBounded(ctx, path+"/"+a.ItemID+"?"+q.Encode(), responseLimit, &item); err != nil {
			return nil, apiError(err)
		}
		if !item.Exportable {
			return nil, errors.New("activity is not exportable")
		}
		return map[string]any{"view": "activity", "item": item, "done": !item.Truncated, "instructions": agentguide.ReferenceInstructions()}, nil
	}
	q := url.Values{"afterPosition": {a.AfterPosition}, "limit": {strconv.Itoa(a.Limit)}, "maxBytesPerItem": {strconv.Itoa(a.MaxBytesPerItem)}}
	var page contract.ConversationActivities
	if err := c.API.GetBounded(ctx, path+"?"+q.Encode(), responseLimit, &page); err != nil {
		return nil, apiError(err)
	}
	items := make([]contract.ConversationActivity, 0, len(page.Items))
	for _, item := range page.Items {
		if item.Exportable {
			items = append(items, item)
		}
	}
	return map[string]any{"view": "activity", "conversationId": session.ConversationID, "items": items, "nextPosition": page.NextPosition, "done": len(page.Items) < a.Limit, "gaps": page.Gaps, "historyOrigin": page.HistoryOrigin, "reconcileRequired": page.ReconcileRequired, "instructions": agentguide.ReferenceInstructions()}, nil
}

var regexpHistoryID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (c Client) readTerminal(ctx context.Context, a readArguments) (any, error) {
	if a.MaxBytes < 1 || a.MaxBytes > 65536 || (a.Generation == "") != (a.Offset == "") {
		return nil, errors.New("generation and offset must be supplied together")
	}
	var offset uint64
	if a.Generation != "" {
		var err error
		offset, err = unsigned(a.Offset)
		if err != nil || !generation.MatchString(a.Generation) {
			return nil, errors.New("invalid terminal cursor")
		}
	}
	q := url.Values{"maxBytes": {strconv.Itoa(a.MaxBytes)}}
	if a.Generation != "" {
		q.Set("generation", a.Generation)
		q.Set("offset", strconv.FormatUint(offset, 10))
	}
	var page contract.TerminalOutput
	if err := c.API.GetBounded(ctx, "/api/sessions/"+url.PathEscape(a.SessionID)+"/output?"+q.Encode(), 256<<10, &page); err != nil {
		return nil, apiError(err)
	}
	return map[string]any{"view": "terminal", "generation": page.Generation, "start": page.Start, "end": page.End, "nextOffset": page.NextOffset, "encoding": page.Encoding, "data": page.Data, "reset": page.Reset, "historyTruncated": page.HistoryTruncated, "limited": page.Limited, "instructions": agentguide.ReferenceInstructions()}, nil
}
