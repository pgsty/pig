package sty

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"pig/internal/config"
	"pig/internal/utils"
)

const bootstrapLocale = "en_US.UTF-8"

var (
	bootLocaleGenFile  = "/etc/locale.gen"
	bootLocaleLookPath = exec.LookPath
	bootLocaleRun      = runBootstrapLocaleCommand
)

// Debian 12 and 13 minimal installations frequently omit generated locales.
// Keep the repair deliberately narrow instead of turning bootstrap into a
// general locale manager.
func needsDebianBootstrapLocale() bool {
	if !strings.EqualFold(strings.TrimSpace(config.OSVendor), "debian") {
		return false
	}
	return config.OSVersion == "12" || config.OSVersion == "13"
}

// repairDebianBootstrapLocale reports whether it changed the locale state.
// This lets Bootstrap distinguish an existing locale from a successful repair
// without probing twice or guessing from the absence of an error.
func repairDebianBootstrapLocale(ctx context.Context) (bool, error) {
	if !needsDebianBootstrapLocale() {
		return false, nil
	}
	if hasDebianBootstrapLocale(ctx) {
		return false, nil
	}
	if err := enableBootstrapLocale(bootLocaleGenFile); err != nil {
		return false, err
	}
	for _, command := range [][]string{
		{"locale-gen", bootstrapLocale},
		{"update-locale", "LANG=" + bootstrapLocale, "LC_ALL=" + bootstrapLocale},
	} {
		if _, err := runBootstrapLocaleStage(ctx, command[0], command[1:]...); err != nil {
			return false, err
		}
	}
	available, err := runBootstrapLocaleStage(ctx, "locale", "-a")
	if err != nil {
		return false, err
	}
	if !containsBootstrapLocale(string(available)) {
		return false, fmt.Errorf("%s was not generated", bootstrapLocale)
	}
	return true, nil
}

func hasDebianBootstrapLocale(ctx context.Context) bool {
	available, err := runBootstrapLocaleStage(ctx, "locale", "-a")
	return err == nil && containsBootstrapLocale(string(available))
}

func enableBootstrapLocale(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing unsafe locale definition file: %s", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	lines := strings.Split(string(content), "\n")
	found, changed := false, false
	for i, line := range lines {
		candidate := strings.TrimSpace(line)
		candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "#"))
		fields := strings.Fields(candidate)
		if len(fields) < 2 || !strings.EqualFold(fields[0], bootstrapLocale) || !strings.EqualFold(fields[1], "UTF-8") {
			continue
		}
		found = true
		if line != bootstrapLocale+" UTF-8" {
			lines[i] = bootstrapLocale + " UTF-8"
			changed = true
		}
		break
	}
	if !found {
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = append(lines[:len(lines)-1], bootstrapLocale+" UTF-8", "")
		} else {
			lines = append(lines, bootstrapLocale+" UTF-8")
		}
		changed = true
	}
	if !changed {
		return nil
	}
	if err := utils.PutFileMode(path, []byte(strings.Join(lines, "\n")), info.Mode().Perm()); err != nil {
		return fmt.Errorf("enable %s in %s: %w", bootstrapLocale, path, err)
	}
	return nil
}

func runBootstrapLocaleStage(ctx context.Context, name string, args ...string) ([]byte, error) {
	path, err := bootLocaleLookPath(name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	output, err := bootLocaleRun(ctx, path, args...)
	if err == nil {
		return output, nil
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return output, fmt.Errorf("%s: %w", name, err)
	}
	return output, fmt.Errorf("%s: %w: %s", name, err, detail)
}

func runBootstrapLocaleCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	env := make([]string, 0, len(os.Environ())+2)
	for _, item := range os.Environ() {
		key := strings.SplitN(item, "=", 2)[0]
		if key == "LANG" || key == "LANGUAGE" || key == "LC_ALL" || strings.HasPrefix(key, "LC_") {
			continue
		}
		env = append(env, item)
	}
	cmd.Env = append(env, "LANG=C", "LC_ALL=C")
	return cmd.CombinedOutput()
}

func containsBootstrapLocale(output string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		return strings.NewReplacer("_", "", "-", "", ".", "").Replace(value)
	}
	want := normalize(bootstrapLocale)
	for _, line := range strings.Split(output, "\n") {
		if normalize(line) == want {
			return true
		}
	}
	return false
}
