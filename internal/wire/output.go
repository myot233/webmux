package wire

import (
	"encoding/base64"
	"strconv"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/session"
)

func TerminalOutput(p session.OutputPage) contract.TerminalOutput {
	return contract.TerminalOutput{Generation: p.Generation, Start: strconv.FormatUint(p.Start, 10), End: strconv.FormatUint(p.End, 10), NextOffset: strconv.FormatUint(p.NextOffset, 10), Encoding: "base64", Data: base64.StdEncoding.EncodeToString(p.Data), Reset: p.Reset, HistoryTruncated: p.HistoryTruncated, Limited: p.Limited}
}
