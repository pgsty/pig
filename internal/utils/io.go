package utils

import (
	"bytes"
	"os"
)

// LimitedBuffer retains at most limit bytes while reporting every input byte
// as consumed, which keeps command pipes bounded without changing writers.
type LimitedBuffer struct {
	bytes.Buffer
	remaining int
}

func NewLimitedBuffer(limit int) *LimitedBuffer {
	return &LimitedBuffer{remaining: max(limit, 0)}
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

func StdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
