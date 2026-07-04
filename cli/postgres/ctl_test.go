/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pg start/stop text-path idempotency (B06/B22, T9 semantics):
starting a running server and stopping a stopped server succeed with a
single informational line instead of failing.
*/
package postgres

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/utils"
)

// stubCtlChecks replaces the ctl state-check seams for one test.
func stubCtlChecks(t *testing.T, exists, initialized, running bool, pid int) {
	t.Helper()
	origDataDir := ctlCheckDataDir
	origRunning := ctlCheckRunning
	origRunningState := ctlCheckRunningState
	t.Cleanup(func() {
		ctlCheckDataDir = origDataDir
		ctlCheckRunning = origRunning
		ctlCheckRunningState = origRunningState
	})
	ctlCheckDataDir = func(dbsu, dataDir string) (bool, bool) { return exists, initialized }
	ctlCheckRunning = func(dbsu, dataDir string) (bool, int) { return running, pid }
	ctlCheckRunningState = func(dbsu, dataDir string) (bool, int, string, error) {
		return running, pid, "", nil
	}
}

func captureCtlStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe failed: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout failed: %v", err)
	}
	_ = r.Close()
	return string(out)
}

func TestStartTextAlreadyRunningIsIdempotentSuccess(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = origFormat })
	config.OutputFormat = config.OUTPUT_TEXT

	stubCtlChecks(t, true, true, true, 4242)

	var startErr error
	out := captureCtlStdout(t, func() {
		startErr = Start(nil, &StartOptions{})
	})
	if startErr != nil {
		t.Fatalf("pg start on running server should succeed, got %v", startErr)
	}
	if !strings.Contains(out, "PostgreSQL is already running (pid 4242)") {
		t.Fatalf("expected already-running line, got %q", out)
	}
}

func TestStartTextUninitializedDataDirStillFails(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = origFormat })
	config.OutputFormat = config.OUTPUT_TEXT

	stubCtlChecks(t, true, false, false, 0)

	err := Start(nil, &StartOptions{})
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("pg start on uninitialized dir should fail, got %v", err)
	}
}

func TestStopTextAlreadyStoppedIsIdempotentSuccess(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = origFormat })
	config.OutputFormat = config.OUTPUT_TEXT

	stubCtlChecks(t, true, true, false, 0)

	var stopErr error
	out := captureCtlStdout(t, func() {
		stopErr = Stop(nil, &StopOptions{Mode: "fast"})
	})
	if stopErr != nil {
		t.Fatalf("pg stop on stopped server should succeed, got %v", stopErr)
	}
	if !strings.Contains(out, "PostgreSQL is already stopped") {
		t.Fatalf("expected already-stopped line, got %q", out)
	}
}

func TestStopTextInvalidModeStillFails(t *testing.T) {
	stubCtlChecks(t, true, true, false, 0)

	err := Stop(nil, &StopOptions{Mode: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "invalid stop mode") {
		t.Fatalf("pg stop with invalid mode should fail, got %v", err)
	}
}

func TestPgStatusSystemdRelatedServicesExcludePostgresUnit(t *testing.T) {
	got := pgStatusRelatedServices()
	want := []string{"patroni", "pgbouncer", "vip-manager", "haproxy"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pgStatusRelatedServices() = %v, want %v", got, want)
	}
	for _, forbidden := range []string{"postgres", "pgbackrest"} {
		for _, service := range got {
			if service == forbidden {
				t.Fatalf("related services should not include %q: %v", forbidden, got)
			}
		}
	}
}

func TestPostgresRuntimeStatusDisplayUsesPostmasterState(t *testing.T) {
	tests := []struct {
		name      string
		running   bool
		wantText  string
		wantColor string
	}{
		{name: "running", running: true, wantText: "up", wantColor: utils.ColorGreen},
		{name: "stopped", running: false, wantText: "down", wantColor: utils.ColorRed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := postgresRuntimeStatusDisplay(tt.running)
			if !got.Show || got.Text != tt.wantText || got.Color != tt.wantColor {
				t.Fatalf("postgresRuntimeStatusDisplay(%v) = %+v, want text=%q color=%q show=true",
					tt.running, got, tt.wantText, tt.wantColor)
			}
		})
	}
}

func TestServiceStatusDisplayMapsSystemdStateToOperatorState(t *testing.T) {
	tests := []struct {
		systemd   string
		wantText  string
		wantColor string
		wantShow  bool
	}{
		{systemd: "active", wantText: "up", wantColor: utils.ColorGreen, wantShow: true},
		{systemd: "inactive", wantText: "down", wantColor: utils.ColorRed, wantShow: true},
		{systemd: "failed", wantText: "down", wantColor: utils.ColorRed, wantShow: true},
		{systemd: "unknown", wantShow: false},
		{systemd: "", wantShow: false},
	}

	for _, tt := range tests {
		t.Run(tt.systemd, func(t *testing.T) {
			got := serviceStatusDisplay(tt.systemd)
			if got.Show != tt.wantShow {
				t.Fatalf("Show = %v, want %v", got.Show, tt.wantShow)
			}
			if !tt.wantShow {
				return
			}
			if got.Text != tt.wantText || got.Color != tt.wantColor {
				t.Fatalf("serviceStatusDisplay(%q) = %+v, want text=%q color=%q",
					tt.systemd, got, tt.wantText, tt.wantColor)
			}
		})
	}
}

func TestRestartTextStoppedFailsInsteadOfStarting(t *testing.T) {
	stubCtlChecks(t, true, true, false, 0)

	err := Restart(nil, &RestartOptions{Mode: "fast"})
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("pg restart on stopped server should fail before pg_ctl restart, got %v", err)
	}
}

func TestRestartTextStatusCheckErrorIsNotReportedAsStopped(t *testing.T) {
	origRunning := ctlCheckRunning
	origRunningState := ctlCheckRunningState
	t.Cleanup(func() {
		ctlCheckRunning = origRunning
		ctlCheckRunningState = origRunningState
	})
	ctlCheckRunning = func(dbsu, dataDir string) (bool, int) {
		return false, 0
	}
	ctlCheckRunningState = func(dbsu, dataDir string) (bool, int, string, error) {
		return false, 0, "", os.ErrPermission
	}

	err := Restart(&Config{PgData: "/pg/data", DbSU: config.CurrentUser}, &RestartOptions{Mode: "fast"})
	if err == nil {
		t.Fatal("expected status check error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("restart should preserve status check error, got %v", err)
	}
	if strings.Contains(err.Error(), "use 'pig pg start'") {
		t.Fatalf("restart should not report permission errors as stopped instance: %v", err)
	}
}

func TestBuildInitDBArgsRendersVersionAwareDefaults(t *testing.T) {
	tests := []struct {
		name            string
		pgVersion       int
		localeAvailable bool
		opts            *InitOptions
		wantArgs        []string
		wantSettings    InitDBSettings
	}{
		{
			name:            "pg16 enables checksums explicitly and uses OS C.UTF-8 when available",
			pgVersion:       16,
			localeAvailable: true,
			opts:            &InitOptions{},
			wantArgs: []string{
				"/pg/bin/initdb",
				"-D", "/pg/data",
				"--encoding=UTF8",
				"--locale=C.UTF-8",
				"--data-checksums",
			},
			wantSettings: InitDBSettings{
				Encoding:      "UTF8",
				Locale:        "C.UTF-8",
				DataChecksums: true,
			},
		},
		{
			name:            "pg17 uses builtin C.UTF-8 and enables checksums explicitly",
			pgVersion:       17,
			localeAvailable: false,
			opts:            &InitOptions{},
			wantArgs: []string{
				"/pg/bin/initdb",
				"-D", "/pg/data",
				"--encoding=UTF8",
				"--locale-provider=builtin",
				"--locale=C.UTF-8",
				"--data-checksums",
			},
			wantSettings: InitDBSettings{
				Encoding:       "UTF8",
				LocaleProvider: "builtin",
				Locale:         "C.UTF-8",
				DataChecksums:  true,
			},
		},
		{
			name:            "pg18 uses default enabled checksums without redundant flag",
			pgVersion:       18,
			localeAvailable: false,
			opts:            &InitOptions{},
			wantArgs: []string{
				"/pg/bin/initdb",
				"-D", "/pg/data",
				"--encoding=UTF8",
				"--locale-provider=builtin",
				"--locale=C.UTF-8",
			},
			wantSettings: InitDBSettings{
				Encoding:       "UTF8",
				LocaleProvider: "builtin",
				Locale:         "C.UTF-8",
				DataChecksums:  true,
			},
		},
		{
			name:            "pg18 renders no-data-checksums only when requested",
			pgVersion:       18,
			localeAvailable: false,
			opts:            &InitOptions{NoDataChecksums: true},
			wantArgs: []string{
				"/pg/bin/initdb",
				"-D", "/pg/data",
				"--encoding=UTF8",
				"--locale-provider=builtin",
				"--locale=C.UTF-8",
				"--no-data-checksums",
			},
			wantSettings: InitDBSettings{
				Encoding:       "UTF8",
				LocaleProvider: "builtin",
				Locale:         "C.UTF-8",
				DataChecksums:  false,
			},
		},
		{
			name:            "pg16 falls back to C with warning when OS C.UTF-8 is unavailable",
			pgVersion:       16,
			localeAvailable: false,
			opts:            &InitOptions{},
			wantArgs: []string{
				"/pg/bin/initdb",
				"-D", "/pg/data",
				"--encoding=UTF8",
				"--locale=C",
				"--data-checksums",
			},
			wantSettings: InitDBSettings{
				Encoding:      "UTF8",
				Locale:        "C",
				DataChecksums: true,
				Warnings: []string{
					"C.UTF-8 locale is unavailable; falling back to C locale",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgs, gotSettings := buildInitDBArgs("/pg/bin/initdb", "/pg/data", tt.pgVersion, tt.opts, tt.localeAvailable)
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Fatalf("args mismatch\nwant: %#v\n got: %#v", tt.wantArgs, gotArgs)
			}
			if !reflect.DeepEqual(gotSettings, tt.wantSettings) {
				t.Fatalf("settings mismatch\nwant: %#v\n got: %#v", tt.wantSettings, gotSettings)
			}
		})
	}
}

func TestBuildInitDBArgsAppendsExtraArgsLast(t *testing.T) {
	args, _ := buildInitDBArgs("/pg/bin/initdb", "/pg/data", 18, &InitOptions{
		NoDataChecksums: true,
		ExtraArgs:       []string{"--waldir=/pg/wal"},
	}, false)

	wantTail := []string{"--no-data-checksums", "--waldir=/pg/wal"}
	gotTail := args[len(args)-len(wantTail):]
	if !reflect.DeepEqual(gotTail, wantTail) {
		t.Fatalf("extra args should remain last\nwant tail: %#v\n got tail: %#v\nall args: %#v", wantTail, gotTail, args)
	}
}

func TestValidateInitOptionsRejectsPolicyOverrides(t *testing.T) {
	tests := []struct {
		name string
		opts *InitOptions
		want string
	}{
		{
			name: "encoding flag",
			opts: &InitOptions{Encoding: "LATIN1"},
			want: "--encoding/-E",
		},
		{
			name: "locale flag",
			opts: &InitOptions{Locale: "en_US.UTF-8"},
			want: "--locale",
		},
		{
			name: "legacy checksum flag",
			opts: &InitOptions{Checksum: true},
			want: "--data-checksum/-k",
		},
		{
			name: "extra encoding long option",
			opts: &InitOptions{ExtraArgs: []string{"--encoding=LATIN1"}},
			want: "--encoding=LATIN1",
		},
		{
			name: "extra encoding short option",
			opts: &InitOptions{ExtraArgs: []string{"-E", "LATIN1"}},
			want: "-E",
		},
		{
			name: "extra locale provider option",
			opts: &InitOptions{ExtraArgs: []string{"--locale-provider=icu"}},
			want: "--locale-provider=icu",
		},
		{
			name: "extra lc option",
			opts: &InitOptions{ExtraArgs: []string{"--lc-collate=C"}},
			want: "--lc-collate=C",
		},
		{
			name: "extra checksum option",
			opts: &InitOptions{ExtraArgs: []string{"--no-data-checksums"}},
			want: "--no-data-checksums",
		},
		{
			name: "extra checksum short option",
			opts: &InitOptions{ExtraArgs: []string{"-k"}},
			want: "-k",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInitOptions(tt.opts)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q should mention %q", err.Error(), tt.want)
			}
			if !strings.Contains(err.Error(), "use initdb directly") {
				t.Fatalf("error should direct users to initdb, got %q", err.Error())
			}
		})
	}
}

func TestValidateInitOptionsAllowsNonPolicyPassthrough(t *testing.T) {
	err := ValidateInitOptions(&InitOptions{
		NoDataChecksums: true,
		ExtraArgs:       []string{"--waldir=/pg/wal", "--auth-local=peer"},
	})
	if err != nil {
		t.Fatalf("non-policy initdb passthrough should be allowed: %v", err)
	}
}
