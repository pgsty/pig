package utils

import "testing"

func TestLimitedBuffer(t *testing.T) {
	buffer := NewLimitedBuffer(4)
	if n, err := buffer.Write([]byte("abcdef")); err != nil || n != 6 {
		t.Fatalf("first write = %d, %v", n, err)
	}
	if n, err := buffer.Write([]byte("gh")); err != nil || n != 2 || buffer.String() != "abcd" {
		t.Fatalf("bounded write = %d, %v, %q", n, err, buffer.String())
	}
}
