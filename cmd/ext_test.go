package cmd

import (
	"testing"

	"pig/internal/output"
)

func TestExtCommandsRejectUnexpectedArgs(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() error
	}{
		{
			name: "status",
			cmd: func() error {
				return extStatusCmd.Args(extStatusCmd, []string{"unexpected"})
			},
		},
		{
			name: "scan",
			cmd: func() error {
				return extScanCmd.Args(extScanCmd, []string{"unexpected"})
			},
		},
		{
			name: "reload",
			cmd: func() error {
				return extReloadCmd.Args(extReloadCmd, []string{"unexpected"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cmd(); err == nil {
				t.Fatalf("%s should reject unexpected args", tt.name)
			}
		})
	}
}

func TestExtAddAndRmRequireTargets(t *testing.T) {
	tests := []struct {
		name string
		args func() error
	}{
		{name: "add", args: func() error { return extAddCmd.Args(extAddCmd, nil) }},
		{name: "rm", args: func() error { return extRmCmd.Args(extRmCmd, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.args(); err == nil {
				t.Fatalf("ext %s should require at least one target", tt.name)
			}
		})
	}
}

func TestExtUpdateMirrorFlagReloadsCatalogFromMirror(t *testing.T) {
	flag := extUpdateCmd.Flags().Lookup("mirror")
	if flag == nil {
		t.Fatal("expected --mirror flag on pig ext update")
	}
	if flag.Shorthand != "m" {
		t.Fatalf("expected shorthand -m for mirror, got -%s", flag.Shorthand)
	}
	if flag.Hidden {
		t.Fatal("pig ext update --mirror should be visible")
	}

	origMirror := flag.Value.String()
	origUpdateMirror := extUpdateMirror
	origYes := extYes
	origProbe := extUpdateProbeVersionExec
	origReload := extUpdateReloadCatalogResultExec
	origLoad := extUpdateLoadCatalogExec
	origUpgrade := extUpdateExec
	defer func() {
		_ = flag.Value.Set(origMirror)
		extUpdateMirror = origUpdateMirror
		extYes = origYes
		extUpdateProbeVersionExec = origProbe
		extUpdateReloadCatalogResultExec = origReload
		extUpdateLoadCatalogExec = origLoad
		extUpdateExec = origUpgrade
	}()

	var gotMirror bool
	var loaded bool
	var gotPgVer int
	var gotNames []string
	extUpdateProbeVersionExec = func() (int, error) {
		return 17, nil
	}
	extUpdateReloadCatalogResultExec = func(mirror bool) *output.Result {
		gotMirror = mirror
		return output.OK("reload ok", nil)
	}
	extUpdateLoadCatalogExec = func(...string) error {
		loaded = true
		return nil
	}
	extUpdateExec = func(pgVer int, names []string, yes bool) *output.Result {
		gotPgVer = pgVer
		gotNames = names
		return output.OK("update ok", nil)
	}

	extYes = true
	if err := flag.Value.Set("true"); err != nil {
		t.Fatalf("set --mirror: %v", err)
	}
	if err := extUpdateCmd.RunE(extUpdateCmd, []string{"postgis"}); err != nil {
		t.Fatalf("pig ext update --mirror failed: %v", err)
	}
	if !gotMirror {
		t.Fatal("expected ext update --mirror to reload catalog from mirror")
	}
	if !loaded {
		t.Fatal("expected ext update --mirror to load reloaded catalog before upgrade")
	}
	if gotPgVer != 17 {
		t.Fatalf("pg version = %d, want 17", gotPgVer)
	}
	if len(gotNames) != 1 || gotNames[0] != "postgis" {
		t.Fatalf("names = %v, want [postgis]", gotNames)
	}
}
