package build

import (
	"pig/cli/ext"
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
			expected: []string{"agensgraph-2.17.0.tar.gz"},
		},
		{
			name:     "openhalo kernel alias",
			pkg:      "openhalo",
			expected: []string{"openhalodb-1.0.tar.gz"},
		},
		{
			name:     "polardb kernel",
			pkg:      "polardb",
			expected: []string{"polardb-for-postgresql-17.10.1.0.tar.gz"},
		},
		{
			name:     "polarstore dependency",
			pkg:      "polarstore",
			expected: []string{"polarstore-1.2.42-d0c5dc6.tar.gz"},
		},
		{
			name:     "zlog dependency",
			pkg:      "zlog",
			expected: []string{"zlog-1.2.18.tar.gz"},
		},
		{
			name:     "ivorysql kernel",
			pkg:      "ivorysql",
			expected: []string{"ivorysql-5.4.tar.gz"},
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
				"Cython-0.29.37.tar.gz",
				"apache-cloudberry-backup-2.1.0-incubating-src.tar.gz",
				"apache-cloudberry-pxf-2.1.0-incubating-src.tar.gz",
				"cloudberry-pxf-2.1.0-rpm-patches.tar.gz",
				"gradle-wrapper-6.8.2.jar",
				"gradle-wrapper-6.8.2.jar.sha256",
				"gradle-wrapper-8.5.jar",
				"gradle-wrapper-8.5.jar.sha256",
				"gradle-6.8.2-bin.zip",
				"gradle-8.5-bin.zip",
			},
		},
		{
			name: "cloudberry pxf bundle",
			pkg:  "cloudberry-pxf",
			expected: []string{
				"apache-cloudberry-pxf-2.1.0-incubating-src.tar.gz",
				"cloudberry-pxf-2.1.0-rpm-patches.tar.gz",
				"gradle-wrapper-6.8.2.jar",
				"gradle-wrapper-6.8.2.jar.sha256",
				"gradle-wrapper-8.5.jar",
				"gradle-wrapper-8.5.jar.sha256",
				"gradle-6.8.2-bin.zip",
				"gradle-8.5-bin.zip",
			},
		},
		{
			name:     "rdkit authoritative bundle",
			pkg:      "rdkit",
			expected: []string{"rdkit_202503.6.orig.tar.xz", "better-enums-0.11.3-enum.h", "rapidjson-1.1.0.tar.gz", "inchi-1.07.3.tar.gz"},
		},
		{
			name: "orioledb defaults to PG18 patchset and extension source",
			pkg:  "orioledb",
			expected: []string{
				"postgres-patches18_1.tar.gz",
				"orioledb-beta16.tar.gz",
			},
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

func TestGetSourceFilesSpecialSourceMappingOrioledbMajorAliases(t *testing.T) {
	tests := []struct {
		pkg      string
		expected []string
	}{
		{
			pkg: "orioledb-16",
			expected: []string{
				"postgres-patches16_47.tar.gz",
				"orioledb-beta16.tar.gz",
			},
		},
		{
			pkg: "orioledb-17",
			expected: []string{
				"postgres-patches17_20.tar.gz",
				"orioledb-beta16.tar.gz",
			},
		},
		{
			pkg: "orioledb-18",
			expected: []string{
				"postgres-patches18_1.tar.gz",
				"orioledb-beta16.tar.gz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			got := getSourceFiles(tt.pkg)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("getSourceFiles(%q) = %v, expected %v", tt.pkg, got, tt.expected)
			}
		})
	}
}

func TestGetSourceFilesSpecialSourceMappingPgedge(t *testing.T) {
	got := getSourceFiles("pgedge")
	want := []string{
		"postgresql-18.4.tar.gz",
		"spock-5.0.10.tar.gz",
		"lolor-1.2.2.tar.gz",
		"snowflake-2.5.0.tar.gz",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("getSourceFiles(%q) = %v, expected %v", "pgedge", got, want)
	}

	hasPostgresSource := false
	hasSpockSource := false
	hasLolorSource := false
	hasSnowflakeSource := false
	for _, src := range got {
		if src == "postgresql-18.4.tar.gz" {
			hasPostgresSource = true
		}
		if src == "spock-5.0.10.tar.gz" {
			hasSpockSource = true
		}
		if src == "lolor-1.2.2.tar.gz" {
			hasLolorSource = true
		}
		if src == "snowflake-2.5.0.tar.gz" {
			hasSnowflakeSource = true
		}
	}

	if !hasPostgresSource || !hasSpockSource || !hasLolorSource || !hasSnowflakeSource {
		t.Fatalf("pgedge sources should include postgres-18, spock, lolor, and snowflake tarballs, got %v", got)
	}
}

func TestGetSourceFilesSpecialSourceMappingPgedgeMajorAliases(t *testing.T) {
	tests := []struct {
		pkg      string
		expected []string
	}{
		{
			pkg: "pgedge-15",
			expected: []string{
				"postgresql-15.18.tar.gz",
				"spock-5.0.10.tar.gz",
				"lolor-1.2.2.tar.gz",
				"snowflake-2.5.0.tar.gz",
			},
		},
		{
			pkg: "pgedge-16",
			expected: []string{
				"postgresql-16.14.tar.gz",
				"spock-5.0.10.tar.gz",
				"lolor-1.2.2.tar.gz",
				"snowflake-2.5.0.tar.gz",
			},
		},
		{
			pkg: "pgedge-17",
			expected: []string{
				"postgresql-17.10.tar.gz",
				"spock-5.0.10.tar.gz",
				"lolor-1.2.2.tar.gz",
				"snowflake-2.5.0.tar.gz",
			},
		},
		{
			pkg: "pgedge-18",
			expected: []string{
				"postgresql-18.4.tar.gz",
				"spock-5.0.10.tar.gz",
				"lolor-1.2.2.tar.gz",
				"snowflake-2.5.0.tar.gz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			got := getSourceFiles(tt.pkg)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("getSourceFiles(%q) = %v, expected %v", tt.pkg, got, tt.expected)
			}
		})
	}
}

func TestGetSourceFilesSpecialSourceMappingBabelfishMajorAliases(t *testing.T) {
	tests := []struct {
		pkg      string
		expected []string
	}{
		{
			pkg:      "babelfish",
			expected: []string{"babelfish-17-17.7-5.4.0.tar.gz"},
		},
		{
			pkg:      "babelfish-17",
			expected: []string{"babelfish-17-17.7-5.4.0.tar.gz"},
		},
		{
			pkg:      "babelfish-18",
			expected: []string{"babelfish-18-18.3-6.0.0.tar.gz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
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
