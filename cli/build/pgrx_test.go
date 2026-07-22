package build

import (
	"fmt"
	"reflect"
	"testing"

	"pig/cli/ext"
)

func TestPgrxAutoDetectVersionStringsAddsBetaOnlyWhenRequested(t *testing.T) {
	oldActive := ext.PostgresActiveMajorVersions
	ext.PostgresActiveMajorVersions = []int{18, 17, 16, 15, 14}
	defer func() {
		ext.PostgresActiveMajorVersions = oldActive
	}()

	wantDefault := []string{"18", "17", "16", "15", "14"}
	if got := pgrxAutoDetectVersionStrings(false); !reflect.DeepEqual(got, wantDefault) {
		t.Fatalf("pgrxAutoDetectVersionStrings(default) = %v, want %v", got, wantDefault)
	}

	wantBeta := []string{fmt.Sprintf("%d", ext.PostgresBetaMajorVersion), "18", "17", "16", "15", "14"}
	if got := pgrxAutoDetectVersionStrings(true); !reflect.DeepEqual(got, wantBeta) {
		t.Fatalf("pgrxAutoDetectVersionStrings(beta) = %v, want %v", got, wantBeta)
	}
}

func TestSplitPgrxVersionStringsKeepsExplicitPgList(t *testing.T) {
	got := splitPgrxVersionStrings("18, 17")
	want := []string{"18", "17"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitPgrxVersionStrings(explicit) = %v, want %v", got, want)
	}
}

func TestBuildLinuxPgrxArgsUsesPlatformPaths(t *testing.T) {
	tests := []struct {
		name  string
		build func(string, []string, bool) []string
		want  []string
	}{
		{
			name:  "EL",
			build: buildELPgrxArgs,
			want:  []string{"cargo", "pgrx", "init", "--pg18=/usr/pgsql-18/bin/pg_config", "--pg17=/usr/pgsql-17/bin/pg_config"},
		},
		{
			name:  "DEB",
			build: buildDEBPgrxArgs,
			want:  []string{"cargo", "pgrx", "init", "--pg18=/usr/lib/postgresql/18/bin/pg_config", "--pg17=/usr/lib/postgresql/17/bin/pg_config"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.build("cargo", []string{"18", "17"}, false); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("build args = %v, want %v", got, tt.want)
			}
		})
	}
}
