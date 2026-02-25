package build

import (
	"reflect"
	"testing"
)

func TestGetSourceFilesSpecialSourceMapping(t *testing.T) {
	tests := []struct {
		name     string
		pkg      string
		expected []string
	}{
		{
			name:     "agensgraph",
			pkg:      "agensgraph",
			expected: []string{"agensgraph-2.16.0.tar.gz"},
		},
		{
			name:     "agentsgraph alias",
			pkg:      "agentsgraph",
			expected: []string{"agensgraph-2.16.0.tar.gz"},
		},
		{
			name:     "pgedge",
			pkg:      "pgedge",
			expected: []string{"postgresql-17.7.tar.gz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSourceFiles(tt.pkg)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("getSourceFiles(%q) = %v, expected %v", tt.pkg, got, tt.expected)
			}
		})
	}
}

func TestGetSourceFilesFallbackToFilename(t *testing.T) {
	pkg := "custom-source-1.0.0.tar.gz"
	got := getSourceFiles(pkg)
	expected := []string{pkg}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("getSourceFiles(%q) = %v, expected %v", pkg, got, expected)
	}
}
