package build

import (
	"pig/cli/ext"
	"reflect"
	"strings"
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
			name:     "pdu alias",
			pkg:      "pdu",
			expected: []string{"pdu-3.0.25.12.tar.gz"},
		},
		{
			name:     "pgdog alias",
			pkg:      "pgdog",
			expected: []string{"pgdog-0.1.32.tar.gz"},
		},
		{
			name: "cloudberry bundle",
			pkg:  "cloudberry",
			expected: []string{
				"apache-cloudberry-2.1.0-incubating-src.tar.gz",
				"psutil-5.7.0.tar.gz",
				"PyGreSQL-5.2.tar.gz",
				"PyYAML-5.4.1.tar.gz",
				"cloudberry-2.1.0-rpm-patches.tar.gz",
				"apache-cloudberry-backup-2.1.0-incubating-src.tar.gz",
				"apache-cloudberry-pxf-2.1.0-incubating-src.tar.gz",
				"cloudberry-pxf-2.1.0-rpm-patches.tar.gz",
			},
		},
		{
			name:     "rdkit authoritative bundle",
			pkg:      "rdkit",
			expected: []string{"rdkit_202503.6.orig.tar.xz", "better-enums-0.11.3-enum.h", "rapidjson-1.1.0.tar.gz", "inchi-1.07.3.tar.gz"},
		},
		{
			name:     "orioledb overrides stale catalog source",
			pkg:      "orioledb",
			expected: []string{"orioledb-beta15.tar.gz"},
		},
		{
			name:     "onesparse alias bundle",
			pkg:      "one_sparse",
			expected: []string{"onesparse-1.0.0.tar.gz", "graphblas-10.2.0.tar.gz", "lagraph-1.2.1.tar.gz"},
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

func TestGetSourceFilesSpecialSourceMappingPgedge(t *testing.T) {
	got := getSourceFiles("pgedge")
	want := []string{"postgresql-18.3.tar.gz", "spock-5.0.6.tar.gz"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("getSourceFiles(%q) = %v, expected %v", "pgedge", got, want)
	}

	hasPostgresSource := false
	hasSpockSource := false
	for _, src := range got {
		if strings.HasPrefix(src, "postgresql-18.") && strings.HasSuffix(src, ".tar.gz") {
			hasPostgresSource = true
		}
		if strings.HasPrefix(src, "spock-5.0.6") && strings.HasSuffix(src, ".tar.gz") {
			hasSpockSource = true
		}
	}

	if !hasPostgresSource || !hasSpockSource {
		t.Fatalf("pgedge sources should include both postgres-18 and spock tarballs, got %v", got)
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

func TestSplitAndCleanSources(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected []string
	}{
		{
			name:     "single source",
			raw:      "foo-1.0.0.tar.gz",
			expected: []string{"foo-1.0.0.tar.gz"},
		},
		{
			name:     "multiple sources with spaces",
			raw:      "foo-1.0.0.tar.gz   bar-2.0.0.tar.gz",
			expected: []string{"foo-1.0.0.tar.gz", "bar-2.0.0.tar.gz"},
		},
		{
			name:     "multiple sources with mixed whitespace",
			raw:      " foo-1.0.0.tar.gz\tbar-2.0.0.tar.gz \nbaz-3.0.0.tar.gz ",
			expected: []string{"foo-1.0.0.tar.gz", "bar-2.0.0.tar.gz", "baz-3.0.0.tar.gz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitAndCleanSources(tt.raw)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("splitAndCleanSources(%q) = %v, expected %v", tt.raw, got, tt.expected)
			}
		})
	}
}

func TestGetSourceFilesFromExtensionSplitSourceField(t *testing.T) {
	const extName = "test_build_get_multi_source"
	const extPkg = "test_build_get_multi_source_pkg"

	originalByName, hadByName := ext.Catalog.ExtNameMap[extName]
	originalByPkg, hadByPkg := ext.Catalog.ExtPkgMap[extPkg]

	ext.Catalog.ExtNameMap[extName] = &ext.Extension{
		Name:   extName,
		Pkg:    extPkg,
		Lead:   true,
		Source: " source-a-1.0.tar.gz\t source-b-2.0.tar.gz ",
	}
	ext.Catalog.ExtPkgMap[extPkg] = ext.Catalog.ExtNameMap[extName]

	t.Cleanup(func() {
		if hadByName {
			ext.Catalog.ExtNameMap[extName] = originalByName
		} else {
			delete(ext.Catalog.ExtNameMap, extName)
		}
		if hadByPkg {
			ext.Catalog.ExtPkgMap[extPkg] = originalByPkg
		} else {
			delete(ext.Catalog.ExtPkgMap, extPkg)
		}
	})

	got := getSourceFiles(extName)
	expected := []string{"source-a-1.0.tar.gz", "source-b-2.0.tar.gz"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("getSourceFiles(%q) = %v, expected %v", extName, got, expected)
	}
}
