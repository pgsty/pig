package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"pig/cli/get"
	"pig/cli/license"
	stycli "pig/cli/sty"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func TestStyDownloadLegacyWrapperStreams(t *testing.T) {
	for _, command := range []*cobra.Command{pigstyInitCmd, pigstyGetcmd} {
		for _, scenario := range []struct {
			name, format string
			corrupt      bool
		}{
			{"json", config.OUTPUT_JSON, false},
			{"text", config.OUTPUT_TEXT, false},
			{"json failure after large diagnostics", config.OUTPUT_JSON, true},
		} {
			t.Run(command.Name()+"/"+scenario.name, func(t *testing.T) {
				payload := bytes.Repeat([]byte("download fixture\n"), 64*1024)
				archive := styDownloadArchive(t, payload)
				checksum := fmt.Sprintf("%x", md5.Sum(archive))
				if scenario.corrupt {
					checksum = strings.Repeat("0", 32)
				}
				var downloads atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/": // Internet-access probe, also redirected to this fixture.
						w.WriteHeader(http.StatusOK)
					case "/src/checksums":
						fmt.Fprintf(w, "%s pigsty-v4.6.0.tgz\n", checksum)
					case "/src/pigsty-v4.6.0.tgz":
						downloads.Add(1)
						w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
						_, _ = w.Write(archive)
					default:
						t.Errorf("unexpected fixture request: %s", r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				t.Cleanup(server.Close)
				setupStyDownloadFixture(t, server)
				config.OutputFormat = scenario.format
				pigstyDownloadDir = t.TempDir()
				pigstyInitPath = filepath.Join(t.TempDir(), "pigsty")
				if scenario.corrupt {
					// Retain a late download failure even after the capture limit.
					// The operation and download below still use the real executors.
					chatter := func() { fmt.Fprint(os.Stderr, strings.Repeat("fixture preparation log\n", 6000)) }
					pigstyInitExec = func(opts stycli.InitOptions) error { chatter(); return stycli.Init(opts) }
					pigstyGetExec = func(opts stycli.DownloadOptions) error { chatter(); return stycli.Download(opts) }
				}

				stdout, stderr, runErr := captureStyDownloadStreams(t, func() error {
					return command.RunE(command, []string{"v4.6.0"})
				})
				if (runErr != nil) != scenario.corrupt || downloads.Load() != 1 {
					t.Fatalf("error=%v downloads=%d, want corrupt=%v and one download", runErr, downloads.Load(), scenario.corrupt)
				}
				if config.OutputFormat != scenario.format {
					t.Fatalf("wrapper did not restore output format: %q", config.OutputFormat)
				}
				if scenario.format == config.OUTPUT_TEXT {
					if stdout != "" || !strings.Contains(stderr, "download pigsty-v4.6.0.tgz [") || !strings.Contains(stderr, "100%") {
						t.Fatalf("text download streams: stdout=%q stderr=%q", stdout, stderr)
					}
				} else {
					var result struct {
						Success bool              `json:"success"`
						Code    int               `json:"code"`
						Detail  string            `json:"detail"`
						Data    legacyCommandData `json:"data"`
					}
					decoder := json.NewDecoder(strings.NewReader(stdout))
					if err := decoder.Decode(&result); err != nil {
						t.Fatalf("stdout is not JSON: %v (%.200s)", err, stdout)
					}
					if err := decoder.Decode(new(interface{})); err != io.EOF {
						t.Fatalf("stdout contains more than one JSON value: %v", err)
					}
					if result.Success == scenario.corrupt || result.Data.Command != "pig sty "+command.Name() || stderr != "" {
						t.Fatalf("result success=%v command=%q stderr=%q", result.Success, result.Data.Command, stderr)
					}
					if strings.ContainsAny(result.Data.CapturedOutput, "\r\x1b") || strings.Contains(result.Data.CapturedOutput, "download pigsty-v4.6.0.tgz [") {
						t.Fatal("transient progress entered captured_output")
					}
					if result.Data.OutputTruncated != scenario.corrupt {
						t.Fatalf("output_truncated=%v, want %v", result.Data.OutputTruncated, scenario.corrupt)
					}
					if scenario.corrupt {
						wantCode := output.CodeStyReleaseDownloadFailed
						if command == pigstyInitCmd {
							wantCode = output.CodeStyInitFailed
						}
						if result.Code != wantCode || !strings.Contains(result.Detail, "checksum mismatch") || !strings.Contains(result.Data.CapturedOutput, "checksum mismatch") {
							t.Fatalf("late download failure lost: code=%d detail=%q", result.Code, result.Detail)
						}
					}
				}

				if scenario.corrupt {
					entries, err := os.ReadDir(pigstyDownloadDir)
					if err != nil || len(entries) != 0 {
						t.Fatalf("failed download left artifacts: %v, %v", entries, err)
					}
					if _, err := os.Stat(pigstyInitPath); !os.IsNotExist(err) {
						t.Fatalf("failed download created install target: %v", err)
					}
					return
				}
				got, err := os.ReadFile(filepath.Join(pigstyDownloadDir, "pigsty-v4.6.0.tgz"))
				if err != nil || !bytes.Equal(got, archive) {
					t.Fatalf("downloaded archive differs: %v", err)
				}
				if command == pigstyInitCmd {
					got, err := os.ReadFile(filepath.Join(pigstyInitPath, "fixture.txt"))
					if err != nil || !bytes.Equal(got, payload) {
						t.Fatalf("real init did not install fixture: %v", err)
					}
				}
				if scenario.format == config.OUTPUT_JSON {
					config.OutputFormat = config.OUTPUT_TEXT
					stdout, stderr, err := captureStyDownloadStreams(t, func() error {
						return get.DownloadSrc("v4.6.0", t.TempDir())
					})
					if err != nil || stdout != "" || !strings.Contains(stderr, "download pigsty-v4.6.0.tgz [") {
						t.Fatalf("wrapper did not restore progress writer: error=%v stdout=%q stderr=%q", err, stdout, stderr)
					}
				}
			})
		}
	}
}

func setupStyDownloadFixture(t *testing.T, server *httptest.Server) {
	t.Helper()
	originalTransport, originalDownloadTransport := http.DefaultTransport, utils.DefaultClient().Transport
	originalVersions, originalLatest := get.AllVersions, get.LatestVersion
	originalSource, originalRegion, originalAccess, originalDetails := get.Source, get.Region, get.InternetAccess, get.Details
	originalFormat, originalManager := config.OutputFormat, license.Manager
	originalInit, originalGet := pigstyInitExec, pigstyGetExec
	originalVersion, originalDir, originalPath := pigstyVersion, pigstyDownloadDir, pigstyInitPath
	originalForce, originalEULA := pigstyInitForce, pigstyInitEULA
	originalInitMirror, originalGetMirror := pigstyInitMirror, pigstyGetMirror
	t.Cleanup(func() {
		http.DefaultTransport, utils.DefaultClient().Transport = originalTransport, originalDownloadTransport
		get.AllVersions, get.LatestVersion = originalVersions, originalLatest
		get.Source, get.Region, get.InternetAccess, get.Details = originalSource, originalRegion, originalAccess, originalDetails
		config.OutputFormat, license.Manager = originalFormat, originalManager
		pigstyInitExec, pigstyGetExec = originalInit, originalGet
		pigstyVersion, pigstyDownloadDir, pigstyInitPath = originalVersion, originalDir, originalPath
		pigstyInitForce, pigstyInitEULA = originalForce, originalEULA
		pigstyInitMirror, pigstyGetMirror = originalInitMirror, originalGetMirror
	})
	t.Cleanup(get.SetDownloadProgressWriter(nil))
	t.Setenv("PATH", t.TempDir()) // No installed Ansible, sudo, or other commands.
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := styDownloadFixtureTransport{base: base, transport: server.Client().Transport}
	http.DefaultTransport, utils.DefaultClient().Transport = transport, transport
	get.AllVersions, get.Details = nil, false
	license.Manager = &license.LicenseManager{}
	pigstyInitExec, pigstyGetExec = stycli.Init, stycli.Download
	pigstyVersion = ""
	pigstyInitForce, pigstyInitEULA, pigstyInitMirror, pigstyGetMirror = false, false, false, false
}

type styDownloadFixtureTransport struct {
	base      *url.URL
	transport http.RoundTripper
}

func (f styDownloadFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	switch request.URL.Host {
	case "repo.pigsty.io", "repo.pigsty.cc", "www.google.com":
	default:
		return nil, fmt.Errorf("unexpected external fixture request: %s", request.URL.Host)
	}
	local := request.Clone(request.Context())
	local.URL.Scheme, local.URL.Host, local.Host = f.base.Scheme, f.base.Host, f.base.Host
	return f.transport.RoundTrip(local)
}

func styDownloadArchive(t *testing.T, payload []byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	// Keep the transfer large enough to exercise repeated progress updates.
	gz, err := gzip.NewWriterLevel(&archive, gzip.NoCompression)
	if err != nil {
		t.Fatal(err)
	}
	tarball := tar.NewWriter(gz)
	if err := tarball.WriteHeader(&tar.Header{Name: "pigsty/fixture.txt", Mode: 0644, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarball.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tarball.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func captureStyDownloadStreams(t *testing.T, run func() error) (string, string, error) {
	t.Helper()
	var streams [2]*os.File
	for i, name := range []string{"stdout", "stderr"} {
		file, err := os.CreateTemp(t.TempDir(), name)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		streams[i] = file
	}
	originalOut, originalErr, originalLog := os.Stdout, os.Stderr, logrus.StandardLogger().Out
	defer func() {
		os.Stdout, os.Stderr = originalOut, originalErr
		logrus.SetOutput(originalLog)
	}()
	os.Stdout, os.Stderr = streams[0], streams[1]
	logrus.SetOutput(os.Stderr)
	runErr := run()
	var captured [2]string
	for i, stream := range streams {
		raw, err := os.ReadFile(stream.Name())
		if err != nil {
			t.Fatal(err)
		}
		captured[i] = string(raw)
	}
	return captured[0], captured[1], runErr
}
