package build

import (
	"pig/cli/ext"
	"testing"
)

func TestShouldUseMakeForKernelPackageShortcut(t *testing.T) {
	tests := []struct {
		name  string
		input string
		ext   *ext.Extension
		want  bool
	}{
		{
			name:  "kernel name itself",
			input: "orioledb",
			ext: &ext.Extension{
				Name:  "orioledb",
				Pkg:   "orioledb",
				Extra: map[string]interface{}{"kernel": "orioledb"},
			},
			want: true,
		},
		{
			name:  "kernel package alias",
			input: "openhalo",
			ext: &ext.Extension{
				Name:  "aux_mysql",
				Pkg:   "openhalo",
				Extra: map[string]interface{}{"kernel": "openhalodb"},
			},
			want: true,
		},
		{
			name:  "kernel-bound component stays regular extension",
			input: "spock",
			ext: &ext.Extension{
				Name:  "spock",
				Pkg:   "spock",
				Extra: map[string]interface{}{"kernel": "pgedge"},
			},
			want: false,
		},
		{
			name:  "regular extension",
			input: "pg_duckdb",
			ext: &ext.Extension{
				Name: "pg_duckdb",
				Pkg:  "pg_duckdb",
			},
			want: false,
		},
		{
			name:  "plain make fallback is not a shortcut",
			input: "pgedge-17",
			ext:   nil,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUseMakeForKernelPackage(tt.input, tt.ext)
			if got != tt.want {
				t.Fatalf("shouldUseMakeForKernelPackage(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveMakeBuildTargetForKernelPackageAlias(t *testing.T) {
	extension := &ext.Extension{
		Name:  "aux_mysql",
		Pkg:   "openhalo",
		Extra: map[string]interface{}{"kernel": "openhalodb"},
	}

	got := resolveMakeBuildTarget("openhalo", extension)
	want := "openhalodb"
	if got != want {
		t.Fatalf("resolveMakeBuildTarget(openhalo) = %q, want %q", got, want)
	}
}
