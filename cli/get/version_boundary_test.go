package get

import "testing"

func TestCompleteVersionComponentBoundaryAndCompatibility(t *testing.T) {
	oldVersions, oldLatest := AllVersions, LatestVersion
	t.Cleanup(func() { AllVersions, LatestVersion = oldVersions, oldLatest })
	AllVersions = []VersionInfo{{Version: "v1.10.3"}, {Version: "v1.1.8"}, {Version: "v1.1.99-rc1"}, {Version: "v1.1.2"}, {Version: "v1.10.9-beta2"}}
	LatestVersion = "v1.10.3"
	for input, want := range map[string]string{
		"1.1": "v1.1.8", "v1.1": "v1.1.8", "1.10": "v1.10.3", "1": "v1.10.3",
		"1.1.2": "v1.1.2", "v1.1.99-rc1": "v1.1.99-rc1", "1.10.9-beta2": "v1.10.9-beta2",
		"latest": "v1.10.3", "1.11": "v1.11", "1.1.77": "v1.1.77", "1.1.77-alpha1": "v1.1.77-alpha1",
	} {
		if got := CompleteVersion(input); got != want {
			t.Errorf("CompleteVersion(%q)=%q, want %q", input, got, want)
		}
	}
}
