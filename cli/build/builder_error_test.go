package build

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"reflect"
	"strings"
	"testing"
	"time"

	"pig/cli/ext"
)

func TestBuildFailureLabels(t *testing.T) {
	tests := []struct {
		name   string
		osType string
		pgVers []int
		builds map[int]*BuildTask
		want   []string
	}{
		{
			name:   "rpm: single failure",
			osType: config.DistroEL,
			pgVers: []int{16, 17},
			builds: map[int]*BuildTask{
				16: {Success: true},
				17: {Success: false},
			},
			want: []string{"PG17"},
		},
		{
			name:   "rpm: missing task treated as failure",
			osType: config.DistroEL,
			pgVers: []int{16, 17},
			builds: map[int]*BuildTask{
				16: {Success: true},
			},
			want: []string{"PG17"},
		},
		{
			name:   "deb: failure",
			osType: config.DistroDEB,
			pgVers: []int{16},
			builds: map[int]*BuildTask{
				0: {Success: false},
			},
			want: []string{"ALL"},
		},
		{
			name:   "deb: missing task treated as failure",
			osType: config.DistroDEB,
			pgVers: []int{16},
			builds: map[int]*BuildTask{},
			want:   []string{"ALL"},
		},
		{
			name:   "deb: success",
			osType: config.DistroDEB,
			pgVers: []int{16},
			builds: map[int]*BuildTask{
				0: {Success: true},
			},
			want: nil,
		},
		{
			name:   "rpm: all success",
			osType: config.DistroEL,
			pgVers: []int{16, 17},
			builds: map[int]*BuildTask{
				16: {Success: true},
				17: {Success: true},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildFailureLabels(tt.osType, tt.pgVers, tt.builds)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildFailureLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindArtifactRejectsStaleAndAcceptsChangedPackage(t *testing.T) {
	home := t.TempDir()
	builder := &ExtensionBuilder{
		OSType:  config.DistroEL,
		OSArch:  "x86_64",
		HomeDir: home,
		Extension: &ext.Extension{
			RpmPkg: "test_$v",
		},
	}
	dir := filepath.Join(home, "rpmbuild", "RPMS", "x86_64")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(dir, "test_17-1.x86_64.rpm")
	if err := os.WriteFile(artifact, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	task := &BuildTask{BeginTime: time.Now(), baseline: builder.snapshotArtifacts([]int{17})}
	if err := builder.findArtifact(17, task); err == nil {
		t.Fatal("stale artifact was accepted")
	}
	if err := os.WriteFile(artifact, []byte("new-package"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := builder.findArtifact(17, task); err != nil {
		t.Fatalf("changed artifact was rejected: %v", err)
	}
	if task.Artifact != artifact || task.Size != int64(len("new-package")) {
		t.Fatalf("artifact result = %q size=%d", task.Artifact, task.Size)
	}
}

func TestFindDebianArtifactsRequiresEveryExpectedVersion(t *testing.T) {
	home := t.TempDir()
	builder := &ExtensionBuilder{
		OSType:     config.DistroDEB,
		HomeDir:    home,
		PGVersions: []int{17, 18},
		Extension: &ext.Extension{
			DebPkg: "postgresql-$v-test",
		},
	}
	task := &BuildTask{baseline: builder.snapshotArtifacts(builder.PGVersions)}
	dir := filepath.Join(home, "ext", "pkg")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(dir, "postgresql-17-test_1_amd64.deb")
	if err := os.WriteFile(artifact, []byte("package"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := builder.findDebianArtifacts(task); err == nil || !strings.Contains(err.Error(), "18") {
		t.Fatalf("partial Debian artifact error = %v", err)
	}
	if task.Artifact != artifact {
		t.Fatalf("partial artifact reporting = %q", task.Artifact)
	}
}

func TestCountNonEmptyLines(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"\n", 0},
		{"a", 1},
		{"a\nb", 2},
		{"a\nb\n", 2},
		{"a\n\nb\n", 2},
		{"  a \n  \n b \n", 2},
	}
	for _, tt := range tests {
		if got := countNonEmptyLines(tt.in); got != tt.want {
			t.Fatalf("countNonEmptyLines(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestDefaultBuilderPGVersions_DebianFiltersByDebPgAndInstalled(t *testing.T) {
	// Preserve globals that defaultBuilderPGVersions depends on.
	oldOSType := config.OSType
	oldOSCode := config.OSCode
	oldOSArch := config.OSArch
	oldDebLib := debianPgLibDir
	t.Cleanup(func() {
		config.OSType = oldOSType
		config.OSCode = oldOSCode
		config.OSArch = oldOSArch
		debianPgLibDir = oldDebLib
	})

	config.OSType = config.DistroDEB
	config.OSCode = "d12"
	config.OSArch = "amd64"

	tmp := t.TempDir()
	debianPgLibDir = tmp
	for _, dir := range []string{"17", "18"} {
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0755); err != nil {
			t.Fatalf("mkdir fake pg dir: %v", err)
		}
	}

	e := &ext.Extension{
		Name:  "pljava",
		PgVer: []string{"18", "17"},
		DebPg: []string{"17"},
	}

	got := defaultBuilderPGVersions(e)
	want := []int{17}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("defaultBuilderPGVersions() = %v, want %v", got, want)
	}
}

func TestNewExtensionBuilderPreservesTargetsAcrossPackageMatrices(t *testing.T) {
	oldOS, oldCode, oldArch := config.OSType, config.OSCode, config.OSArch
	oldCatalog, oldDebLib := ext.Catalog, debianPgLibDir
	t.Cleanup(func() {
		config.OSType, config.OSCode, config.OSArch = oldOS, oldCode, oldArch
		ext.Catalog, debianPgLibDir = oldCatalog, oldDebLib
	})
	debianPgLibDir = t.TempDir()
	for _, version := range []string{"17", "18"} {
		if err := os.Mkdir(filepath.Join(debianPgLibDir, version), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, platform := range []struct{ os, code, arch, matrixPrefix string }{
		{config.DistroEL, "el9", "amd64", "el9i"},
		{config.DistroEL, "el9", "arm64", "el9a"},
		{config.DistroDEB, "d12", "amd64", "d12i"},
		{config.DistroDEB, "d12", "arm64", "d12a"},
	} {
		t.Run(platform.matrixPrefix, func(t *testing.T) {
			config.OSType, config.OSCode, config.OSArch = platform.os, platform.code, platform.arch
			for _, tc := range []struct {
				name      string
				matrix    []string
				supported []string
				want      []int
			}{
				{"no-matrix", nil, []string{"17", "18"}, []int{17, 18}},
				{"both-available", []string{":17:A:f:1:P:1.0", ":18:A:f:1:P:1.0"}, []string{"17", "18"}, []int{17, 18}},
				{"missing-package", []string{":17:A:f:1:P:1.0", ":18:M:f:0::"}, []string{"17", "18"}, []int{17, 18}},
				{"missing-cell", []string{":17:A:f:1:P:1.0"}, []string{"17", "18"}, []int{17, 18}},
				{"empty-matrix", []string{}, []string{"17", "18"}, []int{17, 18}},
				{"all-missing", []string{":17:M:f:0::", ":18:M:f:0::"}, []string{"17", "18"}, []int{17, 18}},
				{"metadata-limit", []string{":17:A:f:1:P:1.0", ":18:A:f:1:P:1.0"}, []string{"17"}, []int{17}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					e := &ext.Extension{Name: "build_probe", Pkg: "build_probe", Lead: true,
						PgVer: []string{"17", "18"}, RpmPg: tc.supported, DebPg: tc.supported}
					if tc.matrix != nil {
						matrix := make([]interface{}, 0, len(tc.matrix))
						for _, cell := range tc.matrix {
							matrix = append(matrix, platform.matrixPrefix+cell)
						}
						e.Extra = map[string]interface{}{"matrix": matrix}
					}
					ext.Catalog = &ext.ExtensionCatalog{ExtNameMap: map[string]*ext.Extension{e.Name: e}}
					builder, err := NewExtensionBuilder(e.Name)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(builder.PGVersions, tc.want) {
						t.Errorf("default build targets = %v, want %v", builder.PGVersions, tc.want)
					}
					if err := builder.UpdateVersion("17,18"); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(builder.PGVersions, []int{17, 18}) {
						t.Errorf("explicit build targets = %v, want [17 18]", builder.PGVersions)
					}
				})
			}
		})
	}
}

func TestBuildVersionForLog(t *testing.T) {
	tests := []struct {
		name   string
		osType string
		ext    *ext.Extension
		want   string
	}{
		{
			name:   "deb prefers DebVer",
			osType: config.DistroDEB,
			ext: &ext.Extension{
				Version: "1.5",
				DebVer:  "1.0",
			},
			want: "1.0",
		},
		{
			name:   "rpm prefers RpmVer",
			osType: config.DistroEL,
			ext: &ext.Extension{
				Version: "1.5",
				RpmVer:  "1.1",
			},
			want: "1.1",
		},
		{
			name:   "fallback to Extension.Version",
			osType: config.DistroDEB,
			ext: &ext.Extension{
				Version: "2.0",
			},
			want: "2.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &ExtensionBuilder{
				OSType:    tt.osType,
				Extension: tt.ext,
			}
			got := b.buildVersionForLog()
			if got != tt.want {
				t.Fatalf("buildVersionForLog() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSourceForLog(t *testing.T) {
	tests := []struct {
		name string
		ext  *ext.Extension
		want string
	}{
		{
			name: "prefer Source field when available",
			ext: &ext.Extension{
				Name:    "aux_mysql",
				Version: "1.5",
				Source:  "openhalodb-1.0.tar.gz",
			},
			want: "openhalodb-1.0.tar.gz",
		},
		{
			name: "keep multiple sources as one raw string",
			ext: &ext.Extension{
				Name:    "pgedge",
				Version: "18.3",
				Source:  "postgresql-18.3.tar.gz spock-5.0.6.tar.gz",
			},
			want: "postgresql-18.3.tar.gz spock-5.0.6.tar.gz",
		},
		{
			name: "empty when Source is missing",
			ext: &ext.Extension{
				Name:    "aux_mysql",
				Version: "1.5",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &ExtensionBuilder{Extension: tt.ext}
			got := b.buildSourceForLog()
			if got != tt.want {
				t.Fatalf("buildSourceForLog() = %q, want %q", got, tt.want)
			}
		})
	}
}
