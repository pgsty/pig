package repo

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUbuntuRepoChannelMapping(t *testing.T) {
	var repos []Repository
	if err := yaml.Unmarshal(embedRepoData, &repos); err != nil {
		t.Fatalf("failed to parse embedded repo catalog: %v", err)
	}

	expectedSuffix := map[string]string{
		"updates":   "-updates",
		"backports": "-backports",
		"security":  "-security",
	}

	// For Ubuntu, we expect two entries (x86_64 + aarch64) for each channel.
	channelCount := map[string]int{
		"updates":   0,
		"backports": 0,
		"security":  0,
	}

	for _, r := range repos {
		if !strings.HasPrefix(r.Description, "Ubuntu ") {
			continue
		}
		suffix, ok := expectedSuffix[r.Name]
		if !ok {
			continue
		}

		channelCount[r.Name]++

		defURL := r.BaseURL["default"]
		chinaURL := r.BaseURL["china"]
		wantFragment := "${distro_codename}" + suffix

		if !strings.Contains(defURL, wantFragment) {
			t.Errorf("ubuntu repo %q default URL mismatch: got %q, want fragment %q", r.Name, defURL, wantFragment)
		}
		if chinaURL != "" && !strings.Contains(chinaURL, wantFragment) {
			t.Errorf("ubuntu repo %q china URL mismatch: got %q, want fragment %q", r.Name, chinaURL, wantFragment)
		}
	}

	for channel, count := range channelCount {
		if count != 2 {
			t.Errorf("expected 2 ubuntu %q entries (x86_64 + aarch64), got %d", channel, count)
		}
	}
}

func TestDebianRepoComponentMapping(t *testing.T) {
	var repos []Repository
	if err := yaml.Unmarshal(embedRepoData, &repos); err != nil {
		t.Fatalf("failed to parse embedded repo catalog: %v", err)
	}

	expectedSuffix := map[string]string{
		"base":     "",
		"updates":  "-updates",
		"security": "-security",
	}
	channelCount := map[string]int{
		"base":     0,
		"updates":  0,
		"security": 0,
	}
	bannedComponents := []string{"restricted", "universe", "multiverse"}

	for _, r := range repos {
		if !strings.HasPrefix(r.Description, "Debian ") {
			continue
		}

		suffix, ok := expectedSuffix[r.Name]
		if !ok {
			continue
		}
		channelCount[r.Name]++

		defURL := r.BaseURL["default"]
		chinaURL := r.BaseURL["china"]
		wantFragment := "${distro_codename}" + suffix

		if !strings.Contains(defURL, wantFragment) {
			t.Errorf("debian repo %q default URL mismatch: got %q, want fragment %q", r.Name, defURL, wantFragment)
		}
		if chinaURL == "" || !strings.Contains(chinaURL, wantFragment) {
			t.Errorf("debian repo %q china URL mismatch: got %q, want fragment %q", r.Name, chinaURL, wantFragment)
		}

		defComponents := aptComponents(defURL)
		chinaComponents := aptComponents(chinaURL)
		if len(defComponents) == 0 || len(chinaComponents) == 0 {
			t.Errorf("debian repo %q failed to parse apt components, default=%q, china=%q", r.Name, defURL, chinaURL)
			continue
		}
		if strings.Join(defComponents, " ") != strings.Join(chinaComponents, " ") {
			t.Errorf("debian repo %q component mismatch: default=%q china=%q", r.Name, strings.Join(defComponents, " "), strings.Join(chinaComponents, " "))
		}

		for _, c := range bannedComponents {
			if slicesContains(defComponents, c) || slicesContains(chinaComponents, c) {
				t.Errorf("debian repo %q should not contain ubuntu component %q: default=%q china=%q", r.Name, c, defURL, chinaURL)
			}
		}
	}

	for channel, count := range channelCount {
		if count != 1 {
			t.Errorf("expected 1 debian %q entry, got %d", channel, count)
		}
	}
}

func aptComponents(repoURL string) []string {
	fields := strings.Fields(repoURL)
	if len(fields) < 3 {
		return nil
	}
	return fields[2:]
}

func slicesContains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
