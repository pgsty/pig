// Package build - deps.go handles build dependency installation
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// InstallDeps installs build dependencies for a single package
func InstallDeps(pkg string, pgVersion string) error {
	logrus.Info(strings.Repeat("=", 58))
	if pgVersion != "" {
		logrus.Infof("[DEPENDENCE] %s (PG%s)", pkg, pgVersion)
	} else {
		logrus.Infof("[DEPENDENCE] %s", pkg)
	}
	logrus.Info(strings.Repeat("=", 58))

	switch config.OSType {
	case config.DistroEL:
		return installRpmDep(pkg, pgVersion)
	case config.DistroDEB:
		return installDebDep(pkg, pgVersion)
	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}
}

// InstallDepsList processes multiple packages
func InstallDepsList(packages []string, pgVersionsStr string) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}

	// Parse PG versions if provided
	var pgVersions []string
	if pgVersionsStr != "" {
		pgVersions = strings.Split(pgVersionsStr, ",")
		for i := range pgVersions {
			pgVersions[i] = strings.TrimSpace(pgVersions[i])
		}
	}

	var failed []string
	for _, pkg := range packages {
		// Check if package is an extension
		_, err := ResolvePackage(pkg)
		isExtension := err == nil

		if isExtension && len(pgVersions) > 0 {
			// For extensions with specified PG versions, install deps for each version
			for _, pgVer := range pgVersions {
				logrus.Infof("Installing deps for extension %s (PG%s)", pkg, pgVer)
				if err := InstallDeps(pkg, pgVer); err != nil {
					logrus.Warnf("Dependency warning for %s (PG%s): %v", pkg, pgVer, err)
					failed = append(failed, fmt.Sprintf("%s(PG%s)", pkg, pgVer))
					// Continue with next version
				}
			}
		} else if isExtension && len(pgVersions) == 0 {
			// For extensions without specified versions, use auto-detection
			if err := InstallDeps(pkg, ""); err != nil {
				logrus.Warnf("Dependency warning for %s: %v", pkg, err)
				failed = append(failed, pkg)
			}
		} else {
			// For non-extension packages, install once regardless of PG versions
			if err := InstallDeps(pkg, ""); err != nil {
				logrus.Warnf("Dependency warning for %s: %v", pkg, err)
				failed = append(failed, pkg)
			}
		}
	}

	if len(failed) > 0 {
		logrus.Warnf("[DEPENDENCE] %d dependency installation warning(s): %s", len(failed), strings.Join(failed, ", "))
	}

	return nil
}

// Install RPM build dependency for single package
func installRpmDep(pkg string, pgVersion string) error {
	specsDir := filepath.Join(config.HomeDir, "rpmbuild", "SPECS")
	if _, err := os.Stat(specsDir); os.IsNotExist(err) {
		return fmt.Errorf("specs directory not found: run 'pig build spec' first")
	}

	// Determine package name and PG version
	var pkgName string
	var pgVer string
	reqPgVer := strings.TrimSpace(pgVersion)
	var extPkg bool

	// Try as extension first
	if ext, err := ResolvePackage(pkg); err == nil {
		pkgName = ext.Pkg
		extPkg = true
		// Use requested PG version first.
		if reqPgVer != "" {
			pgVer = reqPgVer
		} else if len(ext.RpmPg) > 0 {
			// Use extension's highest/first declared RPM PG version.
			pgVer = ext.RpmPg[0]
		}
	} else {
		// Treat as normal package
		pkgName = pkg
	}

	specFile := filepath.Join(specsDir, pkgName+".spec")
	if _, err := os.Stat(specFile); os.IsNotExist(err) {
		return fmt.Errorf("spec file not found for %s: %s", pkgName, specFile)
	}

	if pgVer == "" {
		// Prefer user-provided PG major when available.
		if reqPgVer != "" {
			pgVer = reqPgVer
		} else if inferred := inferRPMPGMajorFromSpec(specFile); inferred != "" {
			pgVer = inferred
		}
	}
	if pgVer == "" {
		// Last-resort fallback for legacy specs without pgmajorversion macro.
		pgVer = "16"
		if extPkg {
			logrus.Warnf("unable to infer PG major for %s, fallback to PG%s", pkgName, pgVer)
		}
	}

	// Install dependencies
	logrus.Infof("install deps for %s (PG%s)", pkgName, pgVer)
	cmd := []string{"dnf", "builddep", "-y", "--define", fmt.Sprintf("pgmajorversion %s", pgVer), specFile}

	if err := utils.SudoCommand(cmd); err != nil {
		return fmt.Errorf("[FAIL] %s build dep missing: %v", pkgName, err)
	}

	logrus.Infof("[DONE] %s build dep complete", pkgName)
	return nil
}

func inferRPMPGMajorFromSpec(specFile string) string {
	content, err := os.ReadFile(specFile)
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if !(strings.HasPrefix(trimmed, "%global pgmajorversion ") || strings.HasPrefix(trimmed, "%define pgmajorversion ")) {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			continue
		}
		if v := leadingDigits(fields[2]); v != "" {
			return v
		}
	}
	return ""
}

// Install DEB build dependency for single package
func installDebDep(pkg string, pgVersion string) error {
	debDir := filepath.Join(config.HomeDir, "debbuild")
	if _, err := os.Stat(debDir); os.IsNotExist(err) {
		return fmt.Errorf("debbuild directory not found: run 'pig build spec' first")
	}

	// Convert package name
	controlFile := filepath.Join(debDir, pkg, "debian", "control.in")
	if _, err := os.Stat(controlFile); os.IsNotExist(err) {
		controlFile = filepath.Join(debDir, pkg, "debian", "control")
		if _, err := os.Stat(controlFile); os.IsNotExist(err) {
			return fmt.Errorf("control file not found for %s: %s", pkg, controlFile)
		}
	}

	// Extract and install dependencies
	content, err := os.ReadFile(controlFile)
	if err != nil {
		return fmt.Errorf("failed to read control file: %v", err)
	}

	deps := parseDebBuildDepends(string(content), pgVersion)
	pgVers, err := resolveDebPGVersionsForDeps(pkg, pgVersion, deps)
	if err != nil {
		return err
	}
	deps = expandDebPGVersionDeps(deps, pgVers)

	if len(deps) == 0 {
		logrus.Warnf("no deb build dependencies parsed for %s", pkg)
		logrus.Infof("[DONE] %s build dep complete", pkg)
		return nil
	}

	logrus.Infof("install deps for %s dependencies", pkg)
	cmd := append([]string{"apt", "install", "-y"}, deps...)

	if err := utils.SudoCommand(cmd); err != nil {
		return fmt.Errorf("[FAIL] %s build dep missing: %v", pkg, err)
	}

	logrus.Infof("[DONE] %s build dep complete", pkg)
	return nil
}

func parseDebBuildDepends(controlContent string, pgVersion string) []string {
	buildDependsFields := []string{
		"Build-Depends",
		"Build-Depends-Arch",
		"Build-Depends-Indep",
	}
	seen := make(map[string]struct{})
	var deps []string

	for _, field := range buildDependsFields {
		buildDepends := extractDebField(controlContent, field)
		if buildDepends == "" {
			continue
		}
		for _, entry := range strings.Split(buildDepends, ",") {
			dep := normalizeDebDependencyEntry(entry, pgVersion)
			if dep == "" {
				continue
			}
			if dep == "postgresql-all" || dep == "debhelper-compat" {
				continue
			}
			if _, ok := seen[dep]; ok {
				continue
			}
			seen[dep] = struct{}{}
			deps = append(deps, dep)
		}
	}

	return deps
}

func resolveDebPGVersionsForDeps(pkg string, pgVersion string, deps []string) ([]string, error) {
	if !containsPGVersionPlaceholder(deps) {
		return nil, nil
	}
	if strings.TrimSpace(pgVersion) != "" {
		return []string{strings.TrimSpace(pgVersion)}, nil
	}

	if ext, err := ResolvePackage(pkg); err == nil {
		installed := intVersionsToStrings(detectInstalledPGVersionsInDir(debianPgLibDir))
		if len(ext.DebPg) > 0 {
			if len(installed) > 0 {
				if inter := intersectStringSets(installed, ext.DebPg); len(inter) > 0 {
					return inter, nil
				}
			}
			return dedupeStrings(ext.DebPg), nil
		}
		if len(installed) > 0 {
			return dedupeStrings(installed), nil
		}
		if len(ext.PgVer) > 0 {
			return dedupeStrings(ext.PgVer), nil
		}
	} else {
		installed := detectInstalledPGVersionsInDir(debianPgLibDir)
		if len(installed) > 0 {
			return intVersionsToStrings(installed), nil
		}
	}

	return nil, fmt.Errorf("[FAIL] %s build dep missing: PGVERSION placeholder exists but no PG version can be resolved (use --pg <major>)", pkg)
}

func containsPGVersionPlaceholder(deps []string) bool {
	for _, dep := range deps {
		if strings.Contains(dep, "PGVERSION") {
			return true
		}
	}
	return false
}

func expandDebPGVersionDeps(deps []string, pgVersions []string) []string {
	if !containsPGVersionPlaceholder(deps) || len(pgVersions) == 0 {
		return dedupeStrings(deps)
	}
	var expanded []string
	for _, dep := range deps {
		if strings.Contains(dep, "PGVERSION") {
			for _, v := range pgVersions {
				v = strings.TrimSpace(v)
				if v == "" {
					continue
				}
				expanded = append(expanded, strings.ReplaceAll(dep, "PGVERSION", v))
			}
			continue
		}
		expanded = append(expanded, dep)
	}
	return dedupeStrings(expanded)
}

func intVersionsToStrings(versions []int) []string {
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		out = append(out, fmt.Sprintf("%d", v))
	}
	return out
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func intersectStringSets(a []string, b []string) []string {
	bSet := make(map[string]struct{}, len(b))
	for _, v := range b {
		v = strings.TrimSpace(v)
		if v != "" {
			bSet[v] = struct{}{}
		}
	}

	var out []string
	seen := make(map[string]struct{})
	for _, v := range a {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := bSet[v]; !ok {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func extractDebField(content string, field string) string {
	fieldPrefix := field + ":"
	lines := strings.Split(content, "\n")
	var value strings.Builder
	capturing := false

	for _, line := range lines {
		if !capturing {
			if strings.HasPrefix(line, fieldPrefix) {
				capturing = true
				head := strings.TrimSpace(strings.TrimPrefix(line, fieldPrefix))
				if head != "" {
					value.WriteString(head)
				}
			}
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if value.Len() > 0 {
				value.WriteString(" ")
			}
			value.WriteString(trimmed)
			continue
		}

		break
	}

	return value.String()
}

func normalizeDebDependencyEntry(entry string, pgVersion string) string {
	dep := strings.TrimSpace(entry)
	if dep == "" {
		return ""
	}

	// Drop version constraints, architecture qualifiers and build profiles.
	dep = stripDelimitedSegments(dep, '(', ')')
	dep = stripDelimitedSegments(dep, '[', ']')
	dep = stripDelimitedSegments(dep, '<', '>')

	// For alternatives like "foo | bar", try the first candidate.
	if idx := strings.Index(dep, "|"); idx >= 0 {
		dep = dep[:idx]
	}

	dep = strings.TrimSpace(strings.Join(strings.Fields(dep), " "))
	if dep == "" {
		return ""
	}

	// Keep package atom only.
	if idx := strings.Index(dep, " "); idx >= 0 {
		dep = dep[:idx]
	}

	if strings.HasPrefix(dep, "${") {
		return ""
	}

	if pgVersion != "" {
		dep = strings.ReplaceAll(dep, "PGVERSION", pgVersion)
	}

	return dep
}

func leadingDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func stripDelimitedSegments(input string, open rune, close rune) string {
	var b strings.Builder
	depth := 0
	for _, r := range input {
		switch {
		case r == open:
			depth++
		case r == close && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
