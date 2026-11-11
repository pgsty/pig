package sty

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"pig/inventory"
)

func TestConfigureNativeRegionPrecedencePigsty46(t *testing.T) {
	for _, test := range []struct {
		name, mode, requested, detected, want string
	}{
		{"explicit default overrides china", "build/oss", "default", "china", "default"},
		{"detected default overrides china", "build/oss", "", "default", "default"},
		{"explicit europe overrides china", "build/oss", "europe", "china", "europe"},
		{"explicit china overrides default", "slim", "china", "default", "china"},
		{"detected china overrides default", "slim", "", "china", "china"},
		{"explicit region wins over detection", "slim", "default", "china", "default"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Cleanup(savePreflightHooks())
			t.Setenv("PATH", t.TempDir())
			detections := 0
			regionDetectFn = func() string {
				detections++
				return test.detected
			}
			home := t.TempDir()
			templatePath := filepath.Join(home, "conf", test.mode+".yml")
			source, err := os.ReadFile(filepath.Join("testdata", "pigsty-4.6", "conf", test.mode+".yml"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(templatePath), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(templatePath, source, 0644); err != nil {
				t.Fatal(err)
			}
			localeAvailable := true
			result := ConfigureNative(ConfigureOptions{
				PigstyHome: home, Mode: test.mode, Region: test.requested,
				Skip: true, NonInteractive: true, DisablePreflight: true,
				CPUCount: 8, LocaleAvailable: &localeAvailable,
			})
			if !result.Success {
				t.Fatalf("ConfigureNative failed: %+v", result)
			}
			data, ok := result.Data.(*ConfigureData)
			if !ok || data.Region != test.want {
				t.Fatalf("reported region = %+v, want %q", result.Data, test.want)
			}
			generated, err := os.ReadFile(data.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			document, err := inventory.Parse(data.OutputPath, generated)
			if err != nil {
				t.Fatal(err)
			}
			region, ok := document.Lookup(inventory.AllVars("region"))
			if !ok || region.Scalar == nil || region.Scalar.Value != data.Region {
				t.Fatalf("generated region = %+v, reported region = %q", region, data.Region)
			}
			unchanged, err := os.ReadFile(templatePath)
			if err != nil || !bytes.Equal(unchanged, source) {
				t.Fatalf("source template changed: %v", err)
			}
			wantDetections := 0
			if test.requested == "" {
				wantDetections = 1
			}
			if detections != wantDetections {
				t.Fatalf("region detection calls = %d, want %d", detections, wantDetections)
			}
		})
	}
}
