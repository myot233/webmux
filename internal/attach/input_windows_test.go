//go:build windows

package attach

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

func TestReadInputWindowsPipeEOF(t *testing.T) {
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var data [1]byte
	n, err := ReadInput(ctx, input, data[:])
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("closed pipe read = %d, %v; want EOF", n, err)
	}
}
func TestReadInputWindowsCancellationIsNotEOF(t *testing.T) {
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var data [1]byte
	if _, err := ReadInput(ctx, input, data[:]); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read = %v", err)
	}
}
