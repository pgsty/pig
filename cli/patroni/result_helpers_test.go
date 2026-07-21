package patroni

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
)

// patroniTestDepsMu serializes package-level hook replacement in tests.
// Do not call t.Parallel in tests that mutate these hooks; t.Cleanup releases
// the lock only when the owning test or subtest completes.
var patroniTestDepsMu sync.Mutex

func argsHas(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func argsHasInOrder(args []string, wants ...string) bool {
	index := 0
	for _, arg := range args {
		if index < len(wants) && arg == wants[index] {
			index++
		}
	}
	return index == len(wants)
}

func withPatroniStdout(t *testing.T, target *bytes.Buffer, fn func()) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	fn()
	_ = writer.Close()
	os.Stdout = oldStdout
	_, _ = io.Copy(target, reader)
	_ = reader.Close()
}
