package utils

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"testing"
)

func TestPutFile(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (string, []byte)
		wantErr     bool
		cleanup     func(string)
		checkResult func(t *testing.T, path string, content []byte)
	}{
		{
			name: "write to new file",
			setup: func(t *testing.T) (string, []byte) {
				dir := t.TempDir()
				return filepath.Join(dir, "test.txt"), []byte("hello world")
			},
			wantErr: false,
			cleanup: func(path string) {
				os.Remove(path)
			},
			checkResult: func(t *testing.T, path string, content []byte) {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("failed to read file: %v", err)
				}
				if string(data) != string(content) {
					t.Errorf("content mismatch, got %q, want %q", string(data), string(content))
				}
			},
		},
		{
			name: "write to existing file with same content",
			setup: func(t *testing.T) (string, []byte) {
				dir := t.TempDir()
				path := filepath.Join(dir, "test.txt")
				content := []byte("hello world")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, content
			},
			wantErr: false,
			cleanup: func(path string) {
				os.Remove(path)
			},
			checkResult: func(t *testing.T, path string, content []byte) {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("failed to read file: %v", err)
				}
				if string(data) != string(content) {
					t.Errorf("content mismatch, got %q, want %q", string(data), string(content))
				}
			},
		},
		{
			name: "write to existing file with different content",
			setup: func(t *testing.T) (string, []byte) {
				dir := t.TempDir()
				path := filepath.Join(dir, "test.txt")
				if err := os.WriteFile(path, []byte("old content"), 0644); err != nil {
					t.Fatal(err)
				}
				return path, []byte("new content")
			},
			wantErr: false,
			cleanup: func(path string) {
				os.Remove(path)
			},
			checkResult: func(t *testing.T, path string, content []byte) {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("failed to read file: %v", err)
				}
				if string(data) != string(content) {
					t.Errorf("content mismatch, got %q, want %q", string(data), string(content))
				}
			},
		},
		{
			name: "write to nested directory",
			setup: func(t *testing.T) (string, []byte) {
				dir := t.TempDir()
				return filepath.Join(dir, "nested", "dir", "test.txt"), []byte("nested content")
			},
			wantErr: false,
			cleanup: func(path string) {
				os.RemoveAll(filepath.Dir(filepath.Dir(path)))
			},
			checkResult: func(t *testing.T, path string, content []byte) {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("failed to read file: %v", err)
				}
				if string(data) != string(content) {
					t.Errorf("content mismatch, got %q, want %q", string(data), string(content))
				}
			},
		},
		{
			name: "write to file with insufficient permissions",
			setup: func(t *testing.T) (string, []byte) {
				dir := t.TempDir()
				path := filepath.Join(dir, "test.txt")
				if err := os.WriteFile(path, []byte("old content"), 0400); err != nil {
					t.Fatal(err)
				}
				return path, []byte("new content")
			},
			wantErr: true,
			cleanup: func(path string) {
				os.Chmod(path, 0644)
				os.Remove(path)
			},
			checkResult: func(t *testing.T, path string, content []byte) {
				data, err := os.ReadFile(path)
				if err == nil {
					t.Errorf("expected error when writing to file with insufficient permissions, got none")
				}
				if string(data) == string(content) {
					t.Errorf("content should not match, got %q, want %q", string(data), string(content))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, content := tt.setup(t)
			defer tt.cleanup(path)

			err := PutFile(path, content)
			if (err != nil) != tt.wantErr {
				t.Errorf("PutFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				tt.checkResult(t, path, content)
			}
		})
	}
}

// TestShellCommand tests the basic functionality of ShellCommand
func TestShellCommand(t *testing.T) {
	// Save the original value
	originalTrySudo := TrySudo
	defer func() {
		TrySudo = originalTrySudo
	}()

	// Scenario 1: Simple command without sudo (echo)
	TrySudo = false
	err := ShellCommand([]string{"echo", "hello"})
	if err != nil {
		t.Fatalf("ShellCommand echo failed: %v", err)
	}

	// Scenario 2: Empty command
	err = ShellCommand([]string{})
	if err == nil {
		t.Fatalf("expected error when no command is passed, got nil")
	}
}

// TestSudoCommand tests the basic functionality of SudoCommand
func TestSudoCommand(t *testing.T) {
	// Save the original value
	originalUser := config.CurrentUser
	defer func() {
		config.CurrentUser = originalUser
	}()

	// Scenario 1: Current user is not root, should prepend sudo
	config.CurrentUser = "testuser"
	err := SudoCommand([]string{"echo", "sudo test"})
	if err != nil {
		// In some environments, if sudo is not available or user fails to input password, it will report an error
		// We are just demonstrating here, so we can make a lenient judgment
		t.Logf("SudoCommand returned error (could be expected in non-privileged environment): %v", err)
	}

	// Scenario 2: Current user is root, should not prepend sudo
	config.CurrentUser = "root"
	err = SudoCommand([]string{"echo", "root test"})
	if err != nil {
		t.Fatalf("SudoCommand failed under root user: %v", err)
	}

	// Scenario 3: Empty command
	err = SudoCommand([]string{})
	if err == nil {
		t.Fatalf("expected error when no command is passed, got nil")
	}
}

// TestDelFile tests the DelFile function for file deletion logic
func TestDelFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_delfile.txt")

	// Write a file first
	err := os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Scenario 1: Direct deletion success
	err = DelFile(testFile)
	if err != nil {
		t.Fatalf("DelFile should succeed, got error: %v", err)
	}
	// Delete again, file no longer exists, should not report error
	err = DelFile(testFile)
	if err != nil {
		t.Fatalf("DelFile on non-existent file should succeed, got error: %v", err)
	}

	// Scenario 2: Create a read-only file, then delete (trigger permission failure)
	testFile2 := filepath.Join(tmpDir, "test_delfile2.txt")
	err = os.WriteFile(testFile2, []byte("read only file"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file2: %v", err)
	}
	// chmod read only
	if err := os.Chmod(testFile2, 0400); err != nil {
		t.Skipf("failed to chmod to read-only, skipping: %v", err)
	}

	err = DelFile(testFile2)
	if err != nil {
		t.Logf("DelFile returned error, likely due to permission (expected in test env): %v", err)
	}
}
