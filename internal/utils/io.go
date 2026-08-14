package utils

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
)

// LimitedBuffer retains at most limit bytes while reporting every input byte
// as consumed, which keeps command pipes bounded without changing writers.
type LimitedBuffer struct {
	bytes.Buffer
	remaining int
}

// TailBuffer retains only the newest bytes written to it. It is useful for
// noisy subprocesses where successful output should stay quiet but a short
// diagnostic tail is still needed on failure.
type TailBuffer struct {
	data  []byte
	limit int
}

// rollingLineWriter mirrors subprocess output onto one terminal line while
// retaining the original stream in tail. Package-manager output commonly uses
// both newlines and carriage returns, so either delimiter advances the rolling
// display without adding permanent log lines.
type rollingLineWriter struct {
	mu        sync.Mutex
	out       io.Writer
	tail      io.Writer
	pending   []byte
	lastWidth int
	active    bool
}

const rollingLineLimit = 72

func NewLimitedBuffer(limit int) *LimitedBuffer {
	return &LimitedBuffer{remaining: max(limit, 0)}
}

func NewTailBuffer(limit int) *TailBuffer {
	return &TailBuffer{limit: max(limit, 0)}
}

func newRollingLineWriter(out, tail io.Writer) *rollingLineWriter {
	return &rollingLineWriter{out: out, tail: tail}
}

func (b *LimitedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	if len(value) > b.remaining {
		value = value[:b.remaining]
	}
	_, _ = b.Buffer.Write(value)
	b.remaining -= len(value)
	return original, nil
}

func (b *TailBuffer) Write(value []byte) (int, error) {
	original := len(value)
	if b.limit == 0 {
		return original, nil
	}
	if len(value) >= b.limit {
		b.data = append(b.data[:0], value[len(value)-b.limit:]...)
		return original, nil
	}
	if overflow := len(b.data) + len(value) - b.limit; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, value...)
	return original, nil
}

func (b *TailBuffer) String() string {
	if b == nil {
		return ""
	}
	return string(b.data)
}

func (w *rollingLineWriter) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.tail != nil {
		_, _ = w.tail.Write(value)
	}
	start := 0
	for i, ch := range value {
		if ch != '\n' && ch != '\r' {
			continue
		}
		w.appendPending(value[start:i])
		w.render(w.pending)
		w.pending = w.pending[:0]
		start = i + 1
	}
	w.appendPending(value[start:])
	if len(w.pending) > 0 {
		w.render(w.pending)
	}
	return len(value), nil
}

func (w *rollingLineWriter) appendPending(value []byte) {
	w.pending = append(w.pending, value...)
	const pendingLimit = 4096
	if len(w.pending) > pendingLimit {
		w.pending = append(w.pending[:0], w.pending[len(w.pending)-pendingLimit:]...)
	}
}

func (w *rollingLineWriter) render(value []byte) {
	if w.out == nil {
		return
	}
	line := strings.Join(strings.Fields(string(value)), " ")
	if line == "" {
		return
	}
	runes := []rune(line)
	if len(runes) > rollingLineLimit {
		runes = append(runes[:rollingLineLimit-3], '.', '.', '.')
		line = string(runes)
	}
	width := len(runes)
	padding := ""
	if width < w.lastWidth {
		padding = strings.Repeat(" ", w.lastWidth-width)
	}
	_, _ = io.WriteString(w.out, "\r"+line+padding)
	w.lastWidth = width
	w.active = true
}

func (w *rollingLineWriter) Finish() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.active || w.out == nil {
		return
	}
	_, _ = io.WriteString(w.out, "\r"+strings.Repeat(" ", w.lastWidth)+"\r")
	w.pending = w.pending[:0]
	w.lastWidth = 0
	w.active = false
}

func StdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
