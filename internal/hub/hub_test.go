package hub

import (
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/session"
)

func TestEncodeSharesRawFrames(t *testing.T) {
	for _, frame := range [][]byte{{0}, {1}, {0, 'h', 'i'}, {1, 'x'}} {
		encoded, exit, err := encode(session.Message{Frame: frame})
		if err != nil || exit || !bytes.Equal(encoded, frame) || &encoded[0] != &frame[0] {
			t.Fatalf("raw frame was rejected, changed or copied: exit=%v err=%v", exit, err)
		}
	}
}

func invalidMessages() map[string]session.Message {
	return map[string]session.Message{
		"empty":       {},
		"empty-raw":   {Frame: []byte{}},
		"raw-control": {Frame: []byte{contract.FrameState, '{', '}'}},
		"unknown-raw": {Frame: []byte{255}},
		"multiple":    {Frame: []byte{contract.FrameOutput, 'x'}, Control: &session.ControlState{}},
		// time.Time implements JSON marshaling and can reject a typed payload.
		"unencodable-state":   {Info: &session.Info{CreatedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}},
		"unencodable-control": {Control: &session.ControlState{Controller: &session.Controller{Since: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}},
	}
}

func TestEncodeRejectsInvalidMessages(t *testing.T) {
	for name, message := range invalidMessages() {
		t.Run(name, func(t *testing.T) {
			frame, _, err := encode(message)
			if err == nil || frame != nil {
				t.Fatalf("invalid message encoded: frame=%x err=%v", frame, err)
			}
		})
	}
}

// writerClient drives the same writer as Attach over a real WebSocket, with a
// controlled queue so invalid producers and precise ordering are reproducible.
func writerClient(t *testing.T, messages []session.Message) (context.Context, *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	sub := &session.Subscriber{Out: make(chan session.Message, len(messages)), Slow: make(chan struct{})}
	for _, message := range messages {
		sub.Out <- message
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = connection.CloseNow() }()
		writeMessages(ctx, NewWebSocketConn(connection), sub, false)
	}))
	t.Cleanup(server.Close)
	client, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	client.SetReadLimit(1024 * 1024)
	t.Cleanup(func() { _ = client.CloseNow() })
	return ctx, client
}

func TestWriterClosesOnInvalidMessage(t *testing.T) {
	for name, message := range invalidMessages() {
		for _, pending := range []bool{false, true} {
			path := "direct"
			if pending {
				path = "after-output"
			}
			t.Run(name+"/"+path, func(t *testing.T) {
				messages := []session.Message{message}
				if pending {
					messages = append([]session.Message{{Frame: []byte{0, 'o', 'k'}}}, messages...)
				}
				ctx, client := writerClient(t, messages)
				if pending {
					kind, frame, err := client.Read(ctx)
					if err != nil || kind != websocket.MessageBinary || !bytes.Equal(frame, []byte{0, 'o', 'k'}) {
						t.Fatalf("valid output preceding error was lost: frame=%x err=%v", frame, err)
					}
				}
				_, _, err := client.Read(ctx)
				var closeError websocket.CloseError
				if !errors.As(err, &closeError) || closeError.Code != websocket.StatusInternalError || closeError.Reason != "invalid session message" {
					t.Fatalf("invalid message did not close safely: %v", err)
				}
			})
		}
	}
}

func TestWriterPreservesBoundariesExitAndSharedChunks(t *testing.T) {
	// Spare capacity is deliberately populated: appending into a subscriber's
	// shared frame would corrupt bytes even if its visible prefix stayed intact.
	storage := bytes.Repeat([]byte{'!'}, 128)
	copy(storage, []byte{0, 'f', 'i', 'r', 's', 't'})
	first := storage[:6]
	before := bytes.Clone(storage)
	second := []byte{0, 's', 'e', 'c', 'o', 'n', 'd'}
	medium := append([]byte{0}, bytes.Repeat([]byte{'M'}, 40000)...)
	large := append([]byte{0}, bytes.Repeat([]byte{'L'}, 70000)...)
	snapshot := []byte{1, 's'}
	last := []byte{0, 't', 'a', 'i', 'l'}
	code := 7
	messages := []session.Message{
		{Frame: first}, {Frame: second}, {Control: &session.ControlState{Cols: 80, Rows: 24}},
		{Frame: medium}, {Frame: large}, {Frame: snapshot}, {Frame: last}, {Exit: &code},
		// Exit is terminal: nothing queued after it may reach the connection.
		{Frame: []byte{0, 'n', 'o'}},
	}
	for range 2 {
		ctx, client := writerClient(t, messages)
		for _, want := range [][]byte{[]byte("\x00firstsecond"), {contract.FrameControl}, medium, large, snapshot, last, {contract.FrameExit}} {
			kind, frame, err := client.Read(ctx)
			if err != nil || kind != websocket.MessageBinary || len(frame) == 0 || frame[0] != want[0] {
				t.Fatalf("frame order changed: frame=%x err=%v, want type %d", frame, err, want[0])
			}
			if len(want) > 1 && !bytes.Equal(frame, want) {
				t.Fatalf("output bytes changed for type %d: got %d bytes, want %d", want[0], len(frame), len(want))
			}
		}
		if _, _, err := client.Read(ctx); websocket.CloseStatus(err) != 4410 {
			t.Fatalf("exit was not the final frame: %v", err)
		}
		if !bytes.Equal(storage, before) || !bytes.Equal(second, []byte("\x00second")) || !bytes.Equal(medium[1:], bytes.Repeat([]byte{'M'}, 40000)) || !bytes.Equal(large[1:], bytes.Repeat([]byte{'L'}, 70000)) {
			t.Fatal("writer modified a shared raw chunk")
		}
	}
}

// A redraw burst arrives as many small PTY reads. The writer sends the first
// at once, then spaces frames about one display frame apart instead of
// forwarding every fragment, without reordering or losing bytes. The stream
// below never pauses for the quiet gap on its own, so only the frame deadline
// can split it: removing the deadline would send it as one frame.
func TestWriterPacesOutputBursts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sub := &session.Subscriber{Out: make(chan session.Message, 4096), Slow: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = connection.CloseNow() }()
		writeMessages(ctx, NewWebSocketConn(connection), sub, false)
	}))
	defer server.Close()
	client, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.CloseNow() }()
	const window = 160 * time.Millisecond
	type produced struct {
		want    []byte
		elapsed time.Duration
		// Gaps longer than the quiet gap let the writer flush early. They come
		// from scheduling, not from the writer, so each may add one frame.
		stalls int
	}
	done := make(chan produced, 1)
	go func() {
		var result produced
		start := time.Now()
		last := start
		for i := 0; time.Since(start) < window; i++ {
			line := fmt.Appendf(nil, "%05d:%s\r\n", i, strings.Repeat("x", 60))
			result.want = append(result.want, line...)
			sub.Out <- session.Message{Frame: append([]byte{contract.FrameOutput}, line...)}
			time.Sleep(time.Millisecond)
			now := time.Now()
			if now.Sub(last) > outputQuietGap {
				result.stalls++
			}
			last = now
		}
		result.elapsed = time.Since(start)
		code := 0
		sub.Out <- session.Message{Exit: &code}
		done <- result
	}()
	var got bytes.Buffer
	frames := 0
	for {
		_, frame, err := client.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if frame[0] == contract.FrameExit {
			break
		}
		if frame[0] != contract.FrameOutput {
			t.Fatalf("unexpected frame %d", frame[0])
		}
		frames++
		got.Write(frame[1:])
	}
	result := <-done
	if !bytes.Equal(got.Bytes(), result.want) {
		t.Fatalf("burst output changed: got %d bytes, want %d", got.Len(), len(result.want))
	}
	paced := int(result.elapsed/outputFrameInterval) + 1
	t.Logf("%d bytes over %s in %d frames (%d frame intervals, %d producer stalls)", got.Len(), result.elapsed, frames, paced, result.stalls)
	// The deadline gives about one frame per interval; stalls only add frames.
	// Waiting for the quiet gap alone would send two frames plus the stalls.
	if frames < paced/2 {
		t.Fatalf("%s of continuous output was sent as %d frames; the frame deadline did not split it", result.elapsed, frames)
	}
	// At most one frame per interval, plus the immediate first frame and
	// one per stall: not one frame per fragment.
	if limit := paced + 1 + result.stalls; frames > limit {
		t.Fatalf("%s of output was sent as %d frames, want at most %d", result.elapsed, frames, limit)
	}
}

// Replay frames are DEFLATEd only when the client asked, only between the
// position and state frames, and live output after the replay stays raw.
func TestWriterDeflatesReplayOnly(t *testing.T) {
	snapshot := append([]byte{contract.FrameSnapshot}, bytes.Repeat([]byte("replayed line\r\n"), 400)...)
	live := append([]byte{contract.FrameOutput}, bytes.Repeat([]byte("live line\r\n"), 400)...)
	for _, ask := range []bool{true, false} {
		t.Run(fmt.Sprint("ask=", ask), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			sub := &session.Subscriber{Out: make(chan session.Message, 8), Slow: make(chan struct{})}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				connection, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer func() { _ = connection.CloseNow() }()
				writeMessages(ctx, NewWebSocketConn(connection), sub, ask)
			}))
			defer server.Close()
			client, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = client.CloseNow() }()
			code := 0
			sub.Out <- session.Message{Position: &session.Position{Mode: "reset"}}
			sub.Out <- session.Message{Frame: snapshot}
			sub.Out <- session.Message{Frame: live}
			sub.Out <- session.Message{Info: &session.Info{}}
			sub.Out <- session.Message{Frame: live}
			sub.Out <- session.Message{Exit: &code}
			var kinds []byte
			for {
				_, frame, err := client.Read(ctx)
				if err != nil {
					t.Fatal(err)
				}
				kinds = append(kinds, frame[0])
				switch frame[0] {
				case contract.FrameDeflate:
					inflated, err := io.ReadAll(flate.NewReader(bytes.NewReader(frame[1:])))
					if err != nil || !bytes.Equal(inflated, snapshot) {
						t.Fatalf("deflated frame does not restore the snapshot: %v", err)
					}
					if len(frame) >= len(snapshot) {
						t.Fatalf("deflated frame is %d bytes, snapshot %d", len(frame), len(snapshot))
					}
				case contract.FrameSnapshot:
					if !bytes.Equal(frame, snapshot) {
						t.Fatal("snapshot changed")
					}
				case contract.FrameOutput:
					if !bytes.Equal(frame, live) {
						t.Fatal("live output changed")
					}
				}
				if frame[0] == contract.FrameExit {
					break
				}
			}
			snapshotKind := contract.FrameSnapshot
			if ask {
				snapshotKind = contract.FrameDeflate
			}
			want := []byte{contract.FramePosition, snapshotKind, contract.FrameOutput, contract.FrameState, contract.FrameOutput, contract.FrameExit}
			if !bytes.Equal(kinds, want) {
				t.Fatalf("frame kinds %v, want %v", kinds, want)
			}
		})
	}
}
