package install

import (
	"fmt"
	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/output"
	"strings"
)

// BuildInstallPlan constructs a structured execution plan for pig install.
// It previews package resolution and command execution without running install.
func BuildInstallPlan(pgVer int, names []string, yes bool, noTranslation bool) *output.Plan {
	command := buildInstallCommand(names, yes, noTranslation)
	if len(names) == 0 {
		return &output.Plan{
			Command:  command,
			Actions:  []output.Action{},
			Affects:  []output.Resource{},
			Expected: "error: no package names provided",
			Risks:    []string{"No package names were provided"},
		}
	}

	if pgVer == 0 {
		pgVer = ext.PostgresLatestMajorVersion
	}

	pkgMgr, err := detectInstallPackageManager()
	if err != nil {
		return &output.Plan{
			Command:  command,
			Actions:  []output.Action{},
			Affects:  []output.Resource{},
			Expected: fmt.Sprintf("error: %v", err),
			Risks:    []string{"No compatible package manager available for current OS"},
		}
	}

	resolved := ext.ResolveInstallPackages(pgVer, names, noTranslation)
	return buildInstallPlanFromState(command, resolved, pkgMgr, yes, noTranslation)
}

func detectInstallPackageManager() (string, error) {
	switch config.OSType {
	case config.DistroEL, config.DistroDEB:
		return ext.PackageManagerCmd(), nil
	case config.DistroMAC:
		return "", fmt.Errorf("macOS brew installation is not supported yet")
	default:
		return "", fmt.Errorf("unsupported OS type: %s", config.OSType)
	}
}

func buildInstallPlanFromState(command string, resolved *ext.PackageResolveResult, pkgMgr string, yes bool, noTranslation bool) *output.Plan {
	actions := buildInstallActions(resolved, pkgMgr, yes)
	affects := buildInstallAffects(resolved)
	expected := buildInstallExpected(resolved)
	risks := buildInstallRisks(resolved, noTranslation)

	return &output.Plan{
		Command:  command,
		Actions:  actions,
		Affects:  affects,
		Expected: expected,
		Risks:    risks,
	}
}

func buildInstallActions(resolved *ext.PackageResolveResult, pkgMgr string, yes bool) []output.Action {
	if resolved == nil {
		return []output.Action{}
	}

	actions := []output.Action{}
	step := 1

	if len(resolved.Packages) > 0 {
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Resolve package names: %s", strings.Join(describeResolvedPackages(resolved), ", ")),
		})
		step++

		yesArg := ""
		if yes {
			yesArg = " -y"
		}
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Execute: sudo %s install%s %s", pkgMgr, yesArg, strings.Join(resolved.Packages, " ")),
		})
		step++
	}

	if len(resolved.NoPackage) > 0 {
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Skip (no package available): %s", strings.Join(resolved.NoPackage, ", ")),
		})
	}

	return actions
}

func describeResolvedPackages(resolved *ext.PackageResolveResult) []string {
	if resolved == nil || len(resolved.Packages) == 0 {
		return []string{}
	}
	details := make([]string, 0, len(resolved.Packages))
	for _, pkg := range resolved.Packages {
		owner := resolved.PackageOwner[pkg]
		if owner != "" && owner != pkg {
			details = append(details, fmt.Sprintf("%s <- %s", pkg, owner))
			continue
		}
		details = append(details, pkg)
	}
	return details
}

func buildInstallAffects(resolved *ext.PackageResolveResult) []output.Resource {
	if resolved == nil {
		return []output.Resource{}
	}
	affects := make([]output.Resource, 0, len(resolved.Packages))
	for _, pkg := range resolved.Packages {
		detail := ""
		if owner := resolved.PackageOwner[pkg]; owner != "" && owner != pkg {
			detail = fmt.Sprintf("requested by %s", owner)
		}
		affects = append(affects, output.Resource{
			Type:   "package",
			Name:   pkg,
			Impact: "install",
			Detail: detail,
		})
	}
	return affects
}

func buildInstallExpected(resolved *ext.PackageResolveResult) string {
	if resolved == nil {
		return "No packages to install"
	}

	parts := make([]string, 0, 2)
	if len(resolved.Packages) > 0 {
		parts = append(parts, fmt.Sprintf("Packages installed: %s", strings.Join(resolved.Packages, ", ")))
	}
	if len(resolved.NoPackage) > 0 {
		parts = append(parts, fmt.Sprintf("Skipped (no package available): %s", strings.Join(resolved.NoPackage, ", ")))
	}
	if len(parts) == 0 {
		return "No packages to install"
	}
	return strings.Join(parts, "; ")
}

func buildInstallRisks(resolved *ext.PackageResolveResult, noTranslation bool) []string {
	risks := make([]string, 0, 2)
	if noTranslation {
		risks = append(risks, "Package translation is disabled; package names are used as-is")
	}
	if resolved != nil && len(resolved.NoPackage) > 0 {
		risks = append(risks, fmt.Sprintf("No package available for: %s", strings.Join(resolved.NoPackage, ", ")))
	}
	if len(risks) == 0 {
		return nil
	}
	return risks
}

func buildInstallCommand(names []string, yes bool, noTranslation bool) string {
	parts := []string{"pig install"}
	if yes {
		parts = append(parts, "-y")
	}
	if noTranslation {
		parts = append(parts, "-n")
	}
	if len(names) > 0 {
		parts = append(parts, strings.Join(names, " "))
	}
	return strings.Join(parts, " ")
}
