package cmd

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"pig/cli/license"
	"pig/internal/config"
	"pig/internal/utils"
)

func TestSensitiveFailureWithholdsErrorDetails(t *testing.T) {
	old := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = old })
	for _, mode := range []string{"json", "yaml"} {
		for _, success := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/success=%v", mode, success), func(t *testing.T) {
				config.OutputFormat = mode
				const secret = "SENTINEL-subprocess-credential"
				var err error
				raw := captureStdout(t, func() {
					err = runLegacyStructuredSensitiveWithNextActions(990000, "sensitive", nil, nil, nil, func() error {
						fmt.Println(secret)
						logrus.Error(secret)
						if success {
							return nil
						}
						return errors.New(secret)
					})
				})
				if (err == nil) != success || strings.Contains(raw, secret) || strings.Contains(fmt.Sprint(err), secret) {
					t.Fatalf("secret in result/error: %q %v", raw, err)
				}
			})
		}
	}
}

func TestLicenseIssueCredentialSurfaces(t *testing.T) {
	oldManager, oldHome := license.Manager, license.HomeDir
	oldKey, oldBy, oldStart, oldType, oldMonth, oldNode := issueKey, issueBy, issueStart, issueType, issueMonth, issueNode
	oldFormat, oldLog, oldLevel := config.OutputFormat, logrus.StandardLogger().Out, logrus.GetLevel()
	t.Cleanup(func() {
		license.Manager, license.HomeDir = oldManager, oldHome
		issueKey, issueBy, issueStart, issueType, issueMonth, issueNode = oldKey, oldBy, oldStart, oldType, oldMonth, oldNode
		config.OutputFormat = oldFormat
		logrus.SetOutput(oldLog)
		logrus.SetLevel(oldLevel)
	})
	license.HomeDir = t.TempDir()
	logrus.SetLevel(logrus.DebugLevel)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	private := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
	publicDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	public := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))
	license.Manager, err = license.NewLicenseManager(public)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherDER, err := x509.MarshalECPrivateKey(otherKey)
	if err != nil {
		t.Fatal(err)
	}
	mismatched := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: otherDER}))
	for _, mode := range []string{"text", "json", "yaml"} {
		for _, outcome := range []string{"invalid", "mismatched", "success"} {
			t.Run(mode+"/"+outcome, func(t *testing.T) {
				success := outcome == "success"
				config.OutputFormat = mode
				issueBy = "fixture"
				issueStart = "2026-10-16"
				issueType = "pro"
				issueMonth = 1
				issueNode = 1
				sentinel := "SENTINEL-invalid-private-key"
				issueKey = sentinel
				if success {
					issueKey = private
					sentinel = strings.Split(private, "\n")[1]
				} else if outcome == "mismatched" {
					issueKey = mismatched
					sentinel = strings.Split(mismatched, "\n")[1]
				}
				var logs bytes.Buffer
				logrus.SetOutput(&logs)
				var runErr error
				raw := captureStdout(t, func() { runErr = licenseIssueCmd.RunE(licenseIssueCmd, []string{"fixture"}) })
				if (runErr == nil) != success {
					t.Fatalf("unexpected result: %v", runErr)
				}
				if mode == "text" && outcome == "mismatched" && !strings.Contains(fmt.Sprint(runErr), "does not match a configured public key") {
					t.Fatalf("mismatched key diagnostic is misleading: %v", runErr)
				}
				all := raw + logs.String() + fmt.Sprint(runErr)
				if strings.Contains(all, sentinel) {
					t.Fatalf("signing credential leaked: %s", all)
				}
				if !success {
					detail := fmt.Sprint(runErr)
					if mode != "text" {
						var result struct {
							Detail string `json:"detail" yaml:"detail"`
						}
						if mode == "json" {
							err = json.Unmarshal([]byte(raw), &result)
						} else {
							err = yaml.Unmarshal([]byte(raw), &result)
						}
						if err != nil {
							t.Fatal(err)
						}
						detail = result.Detail
					}
					want := "invalid or unreadable private key"
					if outcome == "mismatched" {
						want = "does not match a configured public key"
					}
					if !strings.Contains(detail, want) {
						t.Errorf("safe signing-key diagnostic lost: %q, want %q", detail, want)
					}
				}
				if success {
					text := raw
					if mode != "text" {
						var result struct {
							Data legacyCommandData `json:"data" yaml:"data"`
						}
						if mode == "json" {
							err = json.Unmarshal([]byte(raw), &result)
						} else {
							err = yaml.Unmarshal([]byte(raw), &result)
						}
						if err != nil {
							t.Fatal(err)
						}
						text = result.Data.CapturedOutput
					}
					found := false
					for _, line := range strings.Split(text, "\n") {
						if index := strings.Index(line, "raw : "); index >= 0 {
							token := strings.TrimSpace(line[index+len("raw : "):])
							if _, err := license.Manager.Validate(token); err != nil {
								t.Fatalf("issued token is unusable: %v", err)
							}
							if strings.Contains(logs.String(), token) {
								t.Fatal("token leaked to diagnostic log")
							}
							found = true
						}
					}
					if !found {
						t.Fatalf("successful issuance lost its token: %s", raw)
					}
				}
			})
		}
	}
}

func TestLicenseIssuePreservesSafeValidationErrors(t *testing.T) {
	oldFormat, oldKey, oldStart, oldMonth, oldNode := config.OutputFormat, issueKey, issueStart, issueMonth, issueNode
	t.Cleanup(func() {
		config.OutputFormat, issueKey, issueStart, issueMonth, issueNode = oldFormat, oldKey, oldStart, oldMonth, oldNode
	})
	for _, mode := range []string{"text", "json", "yaml"} {
		for _, tc := range []struct {
			name, start, want string
			month, node       int
		}{
			{"month", "2026-10-16", "month limit should be non-negative", -1, 1},
			{"node", "2026-10-16", "node limit should be non-negative", 1, -1},
			{"date", "invalid-date", "should be YYYY-MM-DD", 1, 1},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				config.OutputFormat = mode
				issueKey, issueStart, issueMonth, issueNode = "SENTINEL-unused-signing-key", tc.start, tc.month, tc.node
				var runErr error
				raw := captureStdout(t, func() { runErr = licenseIssueCmd.RunE(licenseIssueCmd, []string{"fixture"}) })
				if runErr == nil || strings.Contains(raw+fmt.Sprint(runErr), issueKey) {
					t.Fatalf("expected failure without signing key exposure: %q, %v", raw, runErr)
				}
				detail := runErr.Error()
				if mode != "text" {
					var result struct {
						Detail string `json:"detail" yaml:"detail"`
					}
					var err error
					if mode == "json" {
						err = json.Unmarshal([]byte(raw), &result)
					} else {
						err = yaml.Unmarshal([]byte(raw), &result)
					}
					if err != nil {
						t.Fatal(err)
					}
					detail = result.Detail
				}
				if !strings.Contains(detail, tc.want) {
					t.Errorf("diagnostic=%q, want %q", detail, tc.want)
				}
			})
		}
	}
}

func TestStructuredLegacyExecutionKeepsNonInteractivePolicy(t *testing.T) {
	oldFormat, oldUser, oldInput := config.OutputFormat, config.CurrentUser, os.Stdin
	t.Cleanup(func() { config.OutputFormat, config.CurrentUser, os.Stdin = oldFormat, oldUser, oldInput })
	config.OutputFormat = config.OUTPUT_JSON
	config.CurrentUser = "dbsu"
	t.Setenv("PIG_NON_INTERACTIVE", "")
	dir := t.TempDir()
	dest := filepath.Join(dir, "input")
	in, err := os.CreateTemp(dir, "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	_, _ = in.WriteString("must not read\n")
	_, _ = in.Seek(0, 0)
	os.Stdin = in
	raw := captureStdout(t, func() {
		if err := runLegacyStructured(990000, "fixture", nil, nil, func() error { return utils.DBSUCommand("dbsu", []string{"sh", "-c", `cat > "$1"`, "fixture", dest}) }); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(raw, `"success":true`) {
		t.Fatalf("result=%s", raw)
	}
	if got, _ := os.ReadFile(dest); len(got) != 0 {
		t.Fatalf("structured wrapper re-enabled stdin: %q", got)
	}
	if os.Getenv("PIG_NON_INTERACTIVE") != "" {
		t.Fatal("wrapper leaked environment state")
	}
}
