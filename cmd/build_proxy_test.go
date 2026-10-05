package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"pig/cli/build"
	"pig/internal/config"
)

func TestBuildProxyRoleGrammarAndDefaults(t *testing.T) {
	for _, name := range []string{"client", "server"} {
		found := false
		for _, c := range buildProxyCmd.Commands() {
			if c.Name() == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("proxy role %s is missing", name)
		}
	}
	client := newBuildProxyClientCommand()
	if f := client.Flags().Lookup("listen"); f == nil || f.DefValue != build.DefaultProxyListen {
		t.Fatal("client does not retain the 12345 default")
	}
	server := newBuildProxyServerCommand()
	if f := server.Flags().Lookup("port"); f == nil || f.DefValue != "443" {
		t.Fatal("public port default is not 443")
	}
	if err := client.Args(client, []string{"one", "two"}); err == nil {
		t.Fatal("client accepted unexpected positionals")
	}
	if err := server.Args(server, []string{"one"}); err == nil {
		t.Fatal("server accepted a positional")
	}
	const secret = "secret-unexpected-credential"
	if err := server.Args(server, []string{secret}); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("unexpected server argument accepted or echoed")
	}
}

func TestBuildProxyClientFailureRedactsURI(t *testing.T) {
	previous := config.OutputFormat
	config.OutputFormat = config.OUTPUT_JSON
	t.Cleanup(func() { config.OutputFormat = previous })
	const secret = "SECRET-UUID-DO-NOT-ECHO"
	client := newBuildProxyClientCommand()
	var runErr error
	raw := captureStdout(t, func() {
		runErr = client.RunE(client, []string{"vless://" + secret + "@proxy.example.com:443?security=reality"})
	})
	if runErr == nil || strings.Contains(raw, secret) {
		t.Fatal("invalid URI succeeded or leaked a credential")
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("failure is not one JSON value: %v", err)
	}
}

func TestBuildProxyExportRejectsStructuredOutputBeforeSetup(t *testing.T) {
	previous := config.OutputFormat
	config.OutputFormat = config.OUTPUT_JSON
	t.Cleanup(func() { config.OutputFormat = previous })
	server := newBuildProxyServerCommand()
	if err := server.Flags().Parse([]string{"--host", "proxy.example.com", "--export-only", "--export", "-"}); err != nil {
		t.Fatal(err)
	}
	var runErr error
	raw := captureStdout(t, func() { runErr = server.RunE(server, nil) })
	if runErr == nil || strings.Contains(raw, "vless://") {
		t.Fatal("structured credential export was accepted")
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("failure is not one JSON value: %v", err)
	}
}
