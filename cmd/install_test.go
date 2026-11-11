package cmd

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/cli/ext"
	"pig/internal/config"
)

func TestInstallPlanFlagExists(t *testing.T) {
	if installCmd.Flags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on install command")
	}
	if installCmd.Flags().Lookup("dry-run") != nil {
		t.Fatal("--dry-run should not exist on install command")
	}
}

func TestInstallRequiresTargets(t *testing.T) {
	if err := installCmd.Args(installCmd, nil); err == nil {
		t.Fatal("install should require at least one target")
	}
}

func TestInstallPlanUsesConfiguredCatalog(t *testing.T) {
	origCatalog := ext.Catalog
	origDir, origOS, origFormat := config.ConfigDir, config.OSType, config.OutputFormat
	origPlan, origNoTranslation, origVersion, origPath := installPlan, installNoTranslation, installPgVer, installPgConfig
	t.Cleanup(func() {
		ext.Catalog = origCatalog
		config.ConfigDir, config.OSType, config.OutputFormat = origDir, origOS, origFormat
		installPlan, installNoTranslation, installPgVer, installPgConfig = origPlan, origNoTranslation, origVersion, origPath
	})
	config.ConfigDir, config.OSType, config.OutputFormat = t.TempDir(), config.DistroEL, config.OUTPUT_JSON
	installPlan, installNoTranslation, installPgVer, installPgConfig = true, false, 18, ""
	var csvData bytes.Buffer
	w := csv.NewWriter(&csvData)
	record := make([]string, 41)
	record[0], record[1], record[2], record[13] = "1", "review_fixture", "review_fixture", "t"
	record[27], record[28] = "review-pg$v", "18"
	if err := w.WriteAll([][]string{make([]string, 41), record}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config.ConfigDir, "extension.csv")
	if err := os.WriteFile(path, csvData.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	raw := captureStdout(t, func() {
		if err := installCmd.RunE(installCmd, []string{"review_fixture"}); err != nil {
			t.Fatal(err)
		}
	})
	if ext.Catalog.DataPath != path || !strings.Contains(raw, "review-pg18") {
		t.Fatalf("install ignored configured catalog: path=%q, result=%s", ext.Catalog.DataPath, raw)
	}
}
