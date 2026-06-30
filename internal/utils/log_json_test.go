package utils

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteLogMessagesJSONLHandlesLongLines(t *testing.T) {
	longMessage := strings.Repeat("x", 128*1024)
	var out bytes.Buffer

	if err := WriteLogMessagesJSONL(&out, "test", strings.NewReader(longMessage+"\n")); err != nil {
		t.Fatalf("WriteLogMessagesJSONL returned error: %v", err)
	}

	var row map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &row); err != nil {
		t.Fatalf("invalid json row: %v", err)
	}
	if row["message"] != longMessage {
		t.Fatalf("message length = %d, want %d", len(row["message"]), len(longMessage))
	}
}
