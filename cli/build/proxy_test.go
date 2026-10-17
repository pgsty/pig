package build

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

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

func TestParseRemoteTargetDoesNotEchoCredential(t *testing.T) {
	const credential = "secret-proxy-credential"
	_, _, _, err := parseRemoteTarget(credential)
	if err == nil {
		t.Fatal("malformed remote target was accepted")
	}
	if strings.Contains(err.Error(), credential) {
		t.Fatalf("remote target error leaked credential: %v", err)
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

func TestProxyInstallCommand(t *testing.T) {
	for _, manager := range []string{"dnf", "yum", "apt-get"} {
		got, err := proxyInstallCommand(manager)
		if err != nil {
			t.Fatalf("proxyInstallCommand(%q) returned error: %v", manager, err)
		}
		want := []string{manager, "install", "-y", "vray"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("proxyInstallCommand(%q) = %#v, want %#v", manager, got, want)
		}
	}
	if command, err := proxyInstallCommand("brew"); err == nil || command != nil {
		t.Fatalf("unsupported package manager result = %#v, %v; want nil and error", command, err)
	}
}

func TestLegacyProxyMalformedInputsWithholdCredentials(t *testing.T) {
	const secret = "SENTINEL-proxy-credential"
	for _, remote := range []string{secret, "id@" + secret, "id@host:" + secret} {
		if _, _, _, err := parseRemoteTarget(remote); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("unsafe remote diagnostic: %v", err)
		}
	}
	for _, local := range []string{secret, "host:" + secret} {
		if _, _, _, err := parseLocalListen(local); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("unsafe local diagnostic: %v", err)
		}
	}
}

func TestProxySubprocessCredentialsStayPrivate(t *testing.T) {
	const secret = "SENTINEL-xray-validator-private-key"
	dir := t.TempDir()
	binary := filepath.Join(dir, "xray-fixture")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s' \"$PIG_TEST_XRAY_SECRET\"\nprintf '%s' \"$PIG_TEST_XRAY_SECRET\" >&2\nexit \"$PIG_TEST_XRAY_EXIT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIG_TEST_XRAY_SECRET", secret)
	external, err := os.CreateTemp(dir, "output")
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr, oldLog := os.Stdout, os.Stderr, logrus.StandardLogger().Out
	var logs bytes.Buffer
	os.Stdout, os.Stderr = external, external
	logrus.SetOutput(&logs)
	t.Cleanup(func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		logrus.SetOutput(oldLog)
		_ = external.Close()
	})
	for _, status := range []string{"0", "17"} {
		t.Run(status, func(t *testing.T) {
			t.Setenv("PIG_TEST_XRAY_EXIT", status)
			private, err := proxyCapture(binary)
			if status == "0" && (err != nil || private != secret) {
				t.Fatalf("private capture did not preserve child stdout: %v", err)
			}
			if status != "0" && (err == nil || private != "" || strings.Contains(err.Error(), secret)) {
				t.Fatalf("failed capture leaked credentials: %v", err)
			}
			r := proxyRuntime{os: "darwin", run: proxyRun}
			err = r.validateConfig(binary, filepath.Join(dir, "candidate.json"))
			if (err == nil) != (status == "0") || strings.Contains(fmt.Sprint(err), secret) {
				t.Fatalf("unsafe validator result: %v", err)
			}
			data, _ := os.ReadFile(external.Name())
			if strings.Contains(string(data)+logs.String(), secret) {
				t.Fatal("private subprocess output escaped to stdout/stderr/logs")
			}
		})
	}
}
