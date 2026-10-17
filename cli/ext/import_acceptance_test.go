package ext

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
)

func TestImportMixedResolutionPreservesCompletedDownloads(t *testing.T) {
	oldCatalog := Catalog
	oldOS, oldCode, oldArch, oldUser := config.OSType, config.OSCode, config.OSArch, config.CurrentUser
	oldMajor := config.OSMajor
	t.Cleanup(func() {
		Catalog = oldCatalog
		config.OSType, config.OSCode, config.OSArch, config.CurrentUser = oldOS, oldCode, oldArch, oldUser
		config.OSMajor = oldMajor
	})
	config.OSType, config.OSCode, config.OSArch, config.CurrentUser, config.OSMajor = config.DistroEL, "el9", "arm64", "root", 9
	e := &Extension{Name: "known", Pkg: "known", RpmPkg: "known_$v", RpmPg: []string{"18"}}
	Catalog = &ExtensionCatalog{ExtNameMap: map[string]*Extension{"known": e}, ExtPkgMap: map[string]*Extension{"known": e}}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "dnf"), []byte("#!/bin/sh\nprintf 'downloaded' > known.rpm\nexit \"$PIG_TEST_DOWNLOAD_EXIT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, exit := range []string{"0", "1"} {
		t.Run(exit, func(t *testing.T) {
			t.Setenv("PIG_TEST_DOWNLOAD_EXIT", exit)
			dir := t.TempDir()
			names := []string{"known", "missing-sentinel"}
			result := ImportExtensionsResult(18, names, dir)
			if result.Success || result.Code != output.CodeExtensionImportFailed || output.ExitCode(result.Code) == 0 {
				t.Fatalf("partial import reported success: %+v", result)
			}
			data := result.Data.(*ImportResultData)
			if !reflect.DeepEqual(data.Requested, names) || !reflect.DeepEqual(data.Packages, []string{"known_18"}) || data.PkgCount != 1 || !reflect.DeepEqual(data.Failed, []string{"missing-sentinel"}) {
				t.Fatalf("partial facts lost: %+v", data)
			}
			if exit == "0" && !reflect.DeepEqual(data.Downloaded, []string{"known_18"}) {
				t.Fatalf("completed download lost: %+v", data)
			}
			if exit == "1" && len(data.Downloaded) != 0 {
				t.Fatalf("failed native command claimed verified downloads: %+v", data)
			}
			if _, err := os.Stat(filepath.Join(dir, "known.rpm")); err != nil {
				t.Fatalf("completed side effect was rolled back: %v", err)
			}
		})
	}
}
