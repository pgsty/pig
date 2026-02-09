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
