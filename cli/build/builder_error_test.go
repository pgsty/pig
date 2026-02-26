package build

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"reflect"
	"testing"

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
				Version: "5.0.5",
				Source:  "postgresql-17.7.tar.gz spock-5.0.5.tar.gz",
			},
			want: "postgresql-17.7.tar.gz spock-5.0.5.tar.gz",
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
