package repo

import (
	"fmt"
	"testing"
)

func TestLoadRepoConfig(t *testing.T) {
	//LoadRpmRepo(nil)
	LoadDebRepo(nil)
	fmt.Println(Repos)
	fmt.Println(RepoMap)

	// print module map
	for k, v := range ModuleMap {
		fmt.Println(k, v)
	}

	//fmt.Println(RepoMap["pigsty-pgsql"].Meta)
	//fmt.Println(RepoMap["pigsty-infra"].Content("default"))
	//fmt.Println(RepoMap["pigsty-pgsql"].Content("default"))

	//AddModule("node")
}

func TestRepoAvailable(t *testing.T) {
	tests := []struct {
		name     string
		repo     Repository
		code     string
		arch     string
		expected bool
	}{
		// EL Tests
		{name: "el8 x86_64 match", repo: Repository{Releases: []int{8}, Arch: []string{"x86_64"}}, code: "el8", arch: "x86_64", expected: true},
		{name: "el8 amd64 match (arch alias)", repo: Repository{Releases: []int{8}, Arch: []string{"x86_64"}}, code: "el8", arch: "amd64", expected: true},
		{name: "el8 arm64 no match", repo: Repository{Releases: []int{8}, Arch: []string{"x86_64"}}, code: "el8", arch: "arm64", expected: false},
		{name: "el7 x86_64 no match (wrong release)", repo: Repository{Releases: []int{8}, Arch: []string{"x86_64"}}, code: "el7", arch: "x86_64", expected: false},
		{name: "multi-arch match", repo: Repository{Releases: []int{8, 9}, Arch: []string{"x86_64", "aarch64"}}, code: "el8", arch: "arm64", expected: true},
		{name: "empty arch matches all", repo: Repository{Releases: []int{8}, Arch: []string{"x86_64"}}, code: "el8", arch: "", expected: true},
		{name: "invalid code format", repo: Repository{Releases: []int{8}, Arch: []string{"x86_64"}}, code: "invalid", arch: "x86_64", expected: false},

		// Ubuntu Tests
		{name: "ubuntu u22 amd64 match", repo: Repository{Releases: []int{22}, Arch: []string{"x86_64"}}, code: "u22", arch: "amd64", expected: true},
		{name: "ubuntu u20 arm64 match", repo: Repository{Releases: []int{20}, Arch: []string{"aarch64"}}, code: "u20", arch: "arm64", expected: true},
		{name: "ubuntu u22 wrong arch", repo: Repository{Releases: []int{22}, Arch: []string{"x86_64"}}, code: "u22", arch: "arm64", expected: false},
		{name: "ubuntu multi-arch match", repo: Repository{Releases: []int{20, 22}, Arch: []string{"x86_64", "aarch64"}}, code: "u20", arch: "arm64", expected: true},

		// Debian Tests
		{name: "debian d11 amd64 match", repo: Repository{Releases: []int{11}, Arch: []string{"x86_64"}}, code: "d11", arch: "amd64", expected: true},
		{name: "debian d12 arm64 match", repo: Repository{Releases: []int{12}, Arch: []string{"aarch64"}}, code: "d12", arch: "arm64", expected: true},
		{name: "debian d11 wrong arch", repo: Repository{Releases: []int{11}, Arch: []string{"x86_64"}}, code: "d11", arch: "arm64", expected: false},
		{name: "debian multi-arch match", repo: Repository{Releases: []int{11, 12}, Arch: []string{"x86_64", "aarch64"}}, code: "d12", arch: "arm64", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.repo.Available(tt.code, tt.arch)
			if result != tt.expected {
				t.Errorf("Available(%s, %s) = %v, want %v", tt.code, tt.arch, result, tt.expected)
			}
		})
	}
}

func TestGetMajorVersionFromCode(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected int
	}{
		// EL Tests
		{name: "valid el7", code: "el7", expected: 7},
		{name: "valid el8", code: "el8", expected: 8},
		{name: "valid el9", code: "el9", expected: 9},
		{name: "invalid el format", code: "elx", expected: -1},
		{name: "el without version", code: "el", expected: -1},

		// Ubuntu Tests
		{name: "ubuntu focal", code: "focal", expected: 20},
		{name: "ubuntu jammy", code: "jammy", expected: 22},
		{name: "invalid ubuntu codename", code: "invalid", expected: -1},

		// Debian Tests
		{name: "debian bullseye", code: "bullseye", expected: 11},
		{name: "debian bookworm", code: "bookworm", expected: 12},
		{name: "debian trixie", code: "trixie", expected: 13},
		{name: "invalid debian codename", code: "invalid", expected: -1},

		// Edge Cases
		{name: "empty string", code: "", expected: -1},
		{name: "numeric only", code: "7", expected: -1},
		{name: "mixed case", code: "EL8", expected: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetMajorVersionFromCode(tt.code)
			if result != tt.expected {
				t.Errorf("GetMajorVersionFromCode(%s) = %v, want %v", tt.code, result, tt.expected)
			}
		})
	}
}
