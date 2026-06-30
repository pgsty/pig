package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// PrintLogMessagesJSONL prints plain log lines as JSONL without command wrappers.
func PrintLogMessagesJSONL(component string, text string) error {
	return WriteLogMessagesJSONL(os.Stdout, component, strings.NewReader(text))
}

func WriteLogMessagesJSONL(w io.Writer, component string, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		row := map[string]string{
			"component": component,
			"message":   scanner.Text(),
		}
		data, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, string(data)); err != nil {
			return err
		}
	}
	return scanner.Err()
}
