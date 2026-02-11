package build

import "testing"

func TestParseRemoteTarget(t *testing.T) {
	userID, host, port, err := parseRemoteTarget("alice@127.0.0.1:8080")
	if err != nil {
		t.Fatalf("parseRemoteTarget returned error: %v", err)
	}
	if userID != "alice" || host != "127.0.0.1" || port != "8080" {
		t.Fatalf("unexpected parse result: userID=%q host=%q port=%q", userID, host, port)
	}
}

func TestParseRemoteTargetInvalid(t *testing.T) {
	tests := []string{
		"alice-127.0.0.1:8080",
		"alice@127.0.0.1",
		"alice@127.0.0.1:abc",
		"alice@127.0.0.1:70000",
	}
	for _, input := range tests {
		if _, _, _, err := parseRemoteTarget(input); err == nil {
			t.Fatalf("expected error for input %q", input)
		}
	}
}

func TestParseLocalListen(t *testing.T) {
	tests := []struct {
		input      string
		wantHost   string
		wantPort   string
		wantListen string
	}{
		{"", "127.0.0.1", "12345", "127.0.0.1:12345"},
		{":2345", "127.0.0.1", "2345", "127.0.0.1:2345"},
		{"*:3456", "0.0.0.0", "3456", "0.0.0.0:3456"},
		{"127.0.0.1:4567", "127.0.0.1", "4567", "127.0.0.1:4567"},
	}
	for _, tt := range tests {
		host, port, listen, err := parseLocalListen(tt.input)
		if err != nil {
			t.Fatalf("parseLocalListen(%q) returned error: %v", tt.input, err)
		}
		if host != tt.wantHost || port != tt.wantPort || listen != tt.wantListen {
			t.Fatalf("parseLocalListen(%q)=%q,%q,%q; want %q,%q,%q",
				tt.input, host, port, listen, tt.wantHost, tt.wantPort, tt.wantListen)
		}
	}
}

func TestParseLocalListenInvalid(t *testing.T) {
	tests := []string{
		"invalid",
		"127.0.0.1",
		"127.0.0.1:abc",
		"127.0.0.1:70000",
	}
	for _, input := range tests {
		if _, _, _, err := parseLocalListen(input); err == nil {
			t.Fatalf("expected error for input %q", input)
		}
	}
}

func TestSetupProxyInvalidLocalNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetupProxy should not panic, got: %v", r)
		}
	}()

	if err := SetupProxy("alice@127.0.0.1:8080", "invalid"); err == nil {
		t.Fatalf("expected SetupProxy to return error on invalid local address")
	}
}
