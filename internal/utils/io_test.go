package utils

import (
	"bytes"
	"strings"
	"testing"
)

func TestLimitedBuffer(t *testing.T) {
	buffer := NewLimitedBuffer(4)
	if n, err := buffer.Write([]byte("abcdef")); err != nil || n != 6 {
		t.Fatalf("first write = %d, %v", n, err)
	}
	if n, err := buffer.Write([]byte("gh")); err != nil || n != 2 || buffer.String() != "abcd" {
		t.Fatalf("bounded write = %d, %v, %q", n, err, buffer.String())
	}
}

func TestTailBufferRetainsNewestBytes(t *testing.T) {
	buffer := NewTailBuffer(6)
	if n, err := buffer.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("first write = %d, %v", n, err)
	}
	if n, err := buffer.Write([]byte("defgh")); err != nil || n != 5 || buffer.String() != "cdefgh" {
		t.Fatalf("rolling write = %d, %v, %q", n, err, buffer.String())
	}
	if n, err := buffer.Write([]byte(strings.Repeat("x", 8))); err != nil || n != 8 || buffer.String() != "xxxxxx" {
		t.Fatalf("oversized write = %d, %v, %q", n, err, buffer.String())
	}
}

func TestRollingLineWriterReplacesNewlinesAndKeepsRawTail(t *testing.T) {
	var display bytes.Buffer
	tail := NewTailBuffer(1024)
	writer := newRollingLineWriter(&display, tail)
	input := "Get:1 package\nGet:2 much-longer-package\rDone\n"
	if n, err := writer.Write([]byte(input)); err != nil || n != len(input) {
		t.Fatalf("rolling write = %d, %v", n, err)
	}
	writer.Finish()
	got := display.String()
	if strings.Contains(got, "\n") || !strings.Contains(got, "\rGet:1 package") ||
		!strings.Contains(got, "\rGet:2 much-longer-package") || !strings.Contains(got, "\rDone") {
		t.Fatalf("rolling display = %q", got)
	}
	if tail.String() != input {
		t.Fatalf("rolling tail = %q, want %q", tail.String(), input)
	}
}

func TestRollingLineWriterJoinsChunkedLinesAndTruncates(t *testing.T) {
	var display bytes.Buffer
	writer := newRollingLineWriter(&display, nil)
	_, _ = writer.Write([]byte("partial"))
	_, _ = writer.Write([]byte(" line\n" + strings.Repeat("x", rollingLineLimit+20)))
	writer.Finish()
	got := display.String()
	if !strings.Contains(got, "\rpartial line") || !strings.Contains(got, "...") || strings.Contains(got, "\n") {
		t.Fatalf("chunked rolling display = %q", got)
	}
}
