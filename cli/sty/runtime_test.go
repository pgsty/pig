package sty

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/cli/get"
	"pig/cli/license"
	"pig/internal/config"
	"pig/internal/output"
)

func TestUsePigstyMirrorSourceRewritesDownloadURLs(t *testing.T) {
	origSource := get.Source
	origRegion := get.Region
	origAllVersions := get.AllVersions
	defer func() {
		get.Source = origSource
		get.Region = origRegion
		get.AllVersions = origAllVersions
	}()

	get.Source = get.ViaIO
	get.Region = "default"
	get.AllVersions = []get.VersionInfo{
		{
			Version:     "v1.2.3",
			Checksum:    "abc",
			DownloadURL: config.RepoPigstyIO + "/src/pigsty-v1.2.3.tgz",
		},
	}

	usePigstyMirrorSource()

	if get.Source != get.ViaCC {
		t.Fatalf("Source = %q, want %q", get.Source, get.ViaCC)
	}
	if get.Region != "china" {
		t.Fatalf("Region = %q, want china", get.Region)
	}
	if got := get.AllVersions[0].DownloadURL; !strings.HasPrefix(got, config.RepoPigstyCC) {
		t.Fatalf("DownloadURL = %q, want prefix %q", got, config.RepoPigstyCC)
	}
	if got := get.AllVersions[0].Checksum; got != "abc" {
		t.Fatalf("Checksum = %q, want abc", got)
	}
}

type versionListTransport struct{}

func (versionListTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	const checksums = "fixture pigsty-v1.10.3.tgz\nfixture pigsty-v1.1.8.tgz\nfixture pigsty-v1.1.99-rc1.tgz\nfixture pigsty-v1.1.2.tgz\n"
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(checksums)), Header: make(http.Header), Request: req}, nil
}

func TestListVersionsPreservesHighestMatchingCompletion(t *testing.T) {
	oldTransport, oldStdout := http.DefaultTransport, os.Stdout
	oldVersions, oldLatest := get.AllVersions, get.LatestVersion
	oldSource, oldRegion, oldAccess, oldDetails := get.Source, get.Region, get.InternetAccess, get.Details
	t.Cleanup(func() {
		http.DefaultTransport, os.Stdout = oldTransport, oldStdout
		get.AllVersions, get.LatestVersion = oldVersions, oldLatest
		get.Source, get.Region, get.InternetAccess, get.Details = oldSource, oldRegion, oldAccess, oldDetails
	})
	http.DefaultTransport = versionListTransport{}
	get.Details = false
	for input, want := range map[string]string{
		"1.1": "v1.10.3,v1.1.99-rc1,v1.1.8", "1.10": "v1.10.3", "1": "v1.10.3",
		"1.1.2": "v1.10.3,v1.1.99-rc1,v1.1.8,v1.1.2", "1.1.99-rc1": "v1.10.3,v1.1.99-rc1",
		"latest": "v1.10.3", "all": "v1.10.3,v1.1.99-rc1,v1.1.8,v1.1.2",
	} {
		t.Run(input, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "versions")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			os.Stdout = file
			err = ListVersionsWithOptions(ListOptions{Args: []string{input}})
			os.Stdout = oldStdout
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(file.Name())
			if err != nil {
				t.Fatal(err)
			}
			var versions []string
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "v") {
					versions = append(versions, strings.Fields(line)[0])
				}
			}
			if got := strings.Join(versions, ","); got != want {
				t.Errorf("sty list %s returned %s, want %s", input, got, want)
			}
		})
	}
}

func TestLifecycleVersionConflictsHaveStableParameterCodes(t *testing.T) {
	for name, err := range map[string]error{
		"init": Init(InitOptions{Version: "v4.4.0", Args: []string{"v4.3.0"}}),
		"get":  Download(DownloadOptions{Version: "v4.4.0", Args: []string{"v4.3.0"}}),
	} {
		want := output.CodeStyInitInvalidArgs
		if name == "get" {
			want = output.CodeStyReleaseInvalidArgs
		}
		if got := lifecycleStatusCode(err); got != want {
			t.Fatalf("%s conflict code = %d, want %d; error=%v", name, got, want, err)
		}
	}
}

func TestLifecycleResourceAndDependencyCodes(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := Deploy(DeployOptions{}); lifecycleStatusCode(err) != output.CodeStyDeployDependencyMissing {
		t.Fatalf("Deploy error = %v", err)
	}
}

func TestLifecycleCommandsRunInHomeWithoutChangingProcessDirectory(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ansible-playbook"), []byte("#!/bin/sh\npwd > deploy.cwd\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"deploy.yml", "pigsty.yml"} {
		if err := os.WriteFile(filepath.Join(home, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	commands := []struct {
		name string
		run  func() error
	}{
		{name: "deploy", run: func() error {
			return Deploy(DeployOptions{PigstyHome: home, PigstyConfig: filepath.Join(home, "pigsty.yml")})
		}},
	}
	for _, command := range commands {
		if err := command.run(); err != nil {
			t.Fatalf("%s: %v", command.name, err)
		}
		if _, err := os.Stat(filepath.Join(home, command.name+".cwd")); err != nil {
			t.Fatalf("%s did not run in Pigsty home: %v", command.name, err)
		}
	}
	after, err := os.Getwd()
	if err != nil || after != before {
		t.Fatalf("working directory changed from %q to %q: %v", before, after, err)
	}
}

func TestProInitRejectsBeforeNetworkOrFilesystemMutation(t *testing.T) {
	originalManager := license.Manager
	originalSource := get.Source
	license.Manager = &license.LicenseManager{Valid: true}
	get.Source = "eula-sentinel"
	t.Cleanup(func() {
		license.Manager = originalManager
		get.Source = originalSource
	})
	t.Setenv("PIG_NON_INTERACTIVE", "1")

	target := filepath.Join(t.TempDir(), "pigsty")
	err := Init(InitOptions{TargetPath: target, Version: "v4.4.0"})
	if got := lifecycleStatusCode(err); got != output.CodeStyInitEULARequired {
		t.Fatalf("Init code = %d, want %d; error=%v", got, output.CodeStyInitEULARequired, err)
	}
	if get.Source != "eula-sentinel" {
		t.Fatalf("network discovery ran before EULA: source=%q", get.Source)
	}
	if _, statErr := os.Lstat(target); !os.IsNotExist(statErr) {
		t.Fatalf("rejected Pro init mutated target: %v", statErr)
	}
}

func lifecycleStatusCode(err error) int {
	var coded interface{ StatusCode() int }
	if errors.As(err, &coded) {
		return coded.StatusCode()
	}
	return 0
}
