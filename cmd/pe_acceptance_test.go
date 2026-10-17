package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
)

func TestExporterHTTPStatusAndPrivateErrors(t *testing.T) {
	oldFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = oldFormat })
	for _, status := range []int{200, 204, 400, 403, 404, 500, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			const private = "pg_secret_response_sentinel"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if status >= 400 {
					_, _ = fmt.Fprint(w, private)
				} else if status != 204 {
					_, _ = fmt.Fprint(w, "# HELP pg_up database status\n# TYPE pg_up gauge\npg_up 1\nother_metric 9\n")
				}
			}))
			defer server.Close()
			host, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
			for _, mode := range []string{"text", "json", "yaml"} {
				for _, action := range []string{"get", "list", "stat", "reload"} {
					t.Run(mode+"/"+action, func(t *testing.T) {
						config.OutputFormat = mode
						command := newPgExporterCommand()
						command.SilenceErrors, command.SilenceUsage = true, true
						command.SetArgs([]string{action, "--host", host, "--port", port})
						var runErr error
						var stderr string
						stdout := captureStdout(t, func() { stderr = captureStderr(t, func() { runErr = command.Execute() }) })
						if (runErr != nil) != (status >= 400) || (utils.ExitCode(runErr) != 0) != (status >= 400) {
							t.Fatalf("HTTP %d returned %v stdout=%s", status, runErr, stdout)
						}
						if strings.Contains(stdout+stderr+fmt.Sprint(runErr), private) {
							t.Fatalf("HTTP error body leaked: stdout=%s stderr=%s error=%v", stdout, stderr, runErr)
						}
						if mode != "text" {
							var result output.Result
							var err error
							if mode == "json" {
								err = json.Unmarshal([]byte(stdout), &result)
							} else {
								err = yaml.Unmarshal([]byte(stdout), &result)
							}
							if err != nil || result.Success != (status < 400) {
								t.Fatalf("invalid structured outcome: %s %v", stdout, err)
							}
						} else if status == 200 {
							if !strings.Contains(stdout, "pg_up") {
								t.Fatalf("normal response lost: %s", stdout)
							}
							if action == "get" && strings.Contains(stdout, "other_metric") {
								t.Fatal("get metric filtering changed")
							}
							if action == "list" && strings.Contains(stdout, "pg_up 1") {
								t.Fatal("list filtering changed")
							}
						}
					})
				}
			}
		})
	}
}
