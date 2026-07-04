package postgres

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseFilesystemProbeHandlesTypedAndUntypedDF(t *testing.T) {
	typed := "Filesystem     Type 1K-blocks Used Available Use% Mounted on\n" +
		"/dev/sdb1      xfs  104857600 1024 104856576   1% /pg\n"
	info, err := parseFilesystemProbe(typed)
	if err != nil {
		t.Fatalf("parse typed df: %v", err)
	}
	if info.Mount != "/pg" || info.Type != "xfs" || info.SizeGB != 100 {
		t.Fatalf("typed probe = %+v, want mount=/pg type=xfs size=100", info)
	}

	untyped := "Filesystem     1K-blocks Used Available Use% Mounted on\n" +
		"/dev/sdc1       41943040 1024 41942016   1% /pg/data\n"
	info, err = parseFilesystemProbe(untyped)
	if err != nil {
		t.Fatalf("parse untyped df: %v", err)
	}
	if info.Mount != "/pg/data" || info.Type != "" || info.SizeGB != 40 {
		t.Fatalf("untyped probe = %+v, want mount=/pg/data type=<empty> size=40", info)
	}
}

func TestDetectDiskGBFallsBackToExistingParent(t *testing.T) {
	orig := filesystemDFOutput
	t.Cleanup(func() {
		filesystemDFOutput = orig
	})

	parent := t.TempDir()
	target := filepath.Join(parent, "missing", "data")
	calls := [][]string{}
	filesystemDFOutput = func(args ...string) (string, error) {
		calls = append(calls, append([]string(nil), args...))
		if len(args) == 2 && args[0] == "-k" && args[1] == target {
			return "", errors.New("missing target")
		}
		if len(args) == 2 && args[0] == "-k" && args[1] == parent {
			return "Filesystem     1K-blocks Used Available Use% Mounted on\n" +
				"/dev/sdc1       41943040 1024 41942016   1% " + parent + "\n", nil
		}
		t.Fatalf("unexpected df args: %#v", args)
		return "", nil
	}

	if got := detectDiskGB(target); got != 40 {
		t.Fatalf("detectDiskGB(%q) = %d, want 40", target, got)
	}
	want := [][]string{{"-k", target}, {"-k", parent}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("df calls = %#v, want %#v", calls, want)
	}
}
