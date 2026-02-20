/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Plan building for ext add/rm commands.
*/
package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"strings"
)

// resolvedExt holds resolved extension info for plan building.
type resolvedExt struct {
	name     string
	ext      *Extension
	packages []string
	alias    bool // true if resolved via AliasMap
}

func resolvePlanExtensions(pgVer int, names []string, parseVersionSpec bool) ([]resolvedExt, []string) {
	Catalog.LoadAliasMap(config.OSType)

	resolved := make([]resolvedExt, 0, len(names))
	notFound := make([]string, 0)

	for _, raw := range names {
		lookupName := raw
		version := ""
		if parseVersionSpec {
			lookupName, version = splitNameVersion(raw, true)
		}

		ext, ok := Catalog.ExtNameMap[lookupName]
		if !ok {
			ext, ok = Catalog.ExtPkgMap[lookupName]
		}
		if !ok {
			// Try alias map.
			if pgPkg, aliasOk, noPackage := resolveAliasPattern(pgVer, lookupName); aliasOk {
				if noPackage {
					notFound = append(notFound, raw)
					continue
				}
				pkgs := ProcessPkgName(pgPkg, pgVer)
				for i, pkg := range pkgs {
					pkgs[i] = applyPackageVersion(pkg, version)
				}
				resolved = append(resolved, resolvedExt{
					name:     lookupName,
					packages: pkgs,
					alias:    true,
				})
				continue
			}
			notFound = append(notFound, raw)
			continue
		}

		pkgName := ext.PackageName(pgVer)
		if pkgName == "" {
			notFound = append(notFound, raw)
			continue
		}

		pkgs := ProcessPkgName(pkgName, pgVer)
		for i, pkg := range pkgs {
			pkgs[i] = applyPackageVersion(pkg, version)
		}
		resolved = append(resolved, resolvedExt{
			name:     ext.Name,
			ext:      ext,
			packages: pkgs,
		})
	}

	return resolved, notFound
}

// ============================================================================
// BuildAddPlan
// ============================================================================

// BuildAddPlan constructs a structured execution plan for ext add.
// It shows what will happen without actually executing the installation.
func BuildAddPlan(pgVer int, names []string, yes bool) *output.Plan {
	if Catalog == nil {
		return &output.Plan{
			Command:  buildAddCommand(names),
			Actions:  []output.Action{},
			Affects:  []output.Resource{},
			Expected: "error: extension catalog not initialized",
			Risks:    []string{"Extension catalog not loaded, cannot resolve packages"},
		}
	}
	if len(names) == 0 {
		return &output.Plan{
			Command:  buildAddCommand(names),
			Actions:  []output.Action{},
			Affects:  []output.Resource{},
			Expected: "error: no extensions specified",
			Risks:    []string{"No extension names were provided"},
		}
	}

	if pgVer == 0 {
		pgVer = PostgresLatestMajorVersion
	}

	resolved, notFound := resolvePlanExtensions(pgVer, names, true)

	// Check if extensions are already installed
	var alreadyInstalled []string
	if Postgres != nil {
		for i := len(resolved) - 1; i >= 0; i-- {
			r := resolved[i]
			if r.ext != nil && Postgres.ExtensionMap[r.name] != nil {
				alreadyInstalled = append(alreadyInstalled, r.name)
				resolved = append(resolved[:i], resolved[i+1:]...)
			}
		}
	}

	return buildAddPlanFromState(names, resolved, notFound, alreadyInstalled, pgVer, yes)
}

// buildAddPlanFromState constructs an add plan from given state.
// This is separated for easier testing.
func buildAddPlanFromState(names []string, resolved []resolvedExt, notFound []string, alreadyInstalled []string, pgVer int, yes bool) *output.Plan {
	// If nothing to install
	if len(resolved) == 0 && len(alreadyInstalled) > 0 {
		return &output.Plan{
			Command:  buildAddCommand(names),
			Actions:  []output.Action{},
			Affects:  []output.Resource{},
			Expected: fmt.Sprintf("No action needed: %s already installed", strings.Join(alreadyInstalled, ", ")),
			Risks:    nil,
		}
	}

	actions := buildAddActions(resolved, notFound, pgVer, yes)
	affects := buildAddAffects(resolved, pgVer)
	expected := buildAddExpected(resolved, alreadyInstalled)
	risks := buildAddRisks(resolved, notFound, alreadyInstalled)

	return &output.Plan{
		Command:  buildAddCommand(names),
		Actions:  actions,
		Affects:  affects,
		Expected: expected,
		Risks:    risks,
	}
}

func buildAddActions(resolved []resolvedExt, notFound []string, pgVer int, yes bool) []output.Action {
	actions := []output.Action{}
	step := 1

	if len(resolved) > 0 {
		// Step: resolve extension names
		var resolveDetails []string
		for _, r := range resolved {
			pkgStr := strings.Join(r.packages, " ")
			resolveDetails = append(resolveDetails, fmt.Sprintf("%s -> %s", r.name, pkgStr))
		}
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Resolve extension names: %s", strings.Join(resolveDetails, ", ")),
		})
		step++

		// Step: execute package manager
		var allPkgs []string
		for _, r := range resolved {
			allPkgs = append(allPkgs, r.packages...)
		}
		pkgMgr := PackageManagerCmd()
		yesArg := ""
		if yes {
			yesArg = " -y"
		}
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Execute: sudo %s install%s %s", pkgMgr, yesArg, strings.Join(allPkgs, " ")),
		})
		step++
	}

	if len(notFound) > 0 {
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Skip (not found): %s", strings.Join(notFound, ", ")),
		})
	}

	return actions
}

func buildAddAffects(resolved []resolvedExt, pgVer int) []output.Resource {
	affects := []output.Resource{}

	for _, r := range resolved {
		for _, pkg := range r.packages {
			detail := ""
			if r.ext != nil {
				detail = r.ext.EnDesc
			}
			affects = append(affects, output.Resource{
				Type:   "package",
				Name:   pkg,
				Impact: "install",
				Detail: detail,
			})
		}
	}

	// Check if any extension needs shared_preload_libraries
	for _, r := range resolved {
		if r.ext != nil && r.ext.NeedLoad {
			affects = append(affects, output.Resource{
				Type:   "service",
				Name:   fmt.Sprintf("postgresql-%d", pgVer),
				Impact: "may require restart",
				Detail: fmt.Sprintf("extension %s needs shared_preload_libraries", r.name),
			})
			break // Only add service entry once
		}
	}

	return affects
}

func buildAddExpected(resolved []resolvedExt, alreadyInstalled []string) string {
	var parts []string
	if len(resolved) > 0 {
		var names []string
		for _, r := range resolved {
			names = append(names, r.name)
		}
		parts = append(parts, fmt.Sprintf("Extensions installed: %s", strings.Join(names, ", ")))
	}
	if len(alreadyInstalled) > 0 {
		parts = append(parts, fmt.Sprintf("Already installed: %s", strings.Join(alreadyInstalled, ", ")))
	}
	if len(parts) == 0 {
		return "No extensions to install"
	}
	return strings.Join(parts, "; ")
}

func buildAddRisks(resolved []resolvedExt, notFound []string, alreadyInstalled []string) []string {
	var risks []string

	// NeedLoad risk
	for _, r := range resolved {
		if r.ext != nil && r.ext.NeedLoad {
			risks = append(risks, fmt.Sprintf("Extension %s requires shared_preload_libraries configuration and PostgreSQL restart", r.name))
		}
	}

	// Not found risk
	if len(notFound) > 0 {
		risks = append(risks, fmt.Sprintf("Extensions not found in catalog: %s", strings.Join(notFound, ", ")))
	}

	return risks
}

func buildAddCommand(names []string) string {
	return fmt.Sprintf("pig ext add %s", strings.Join(names, " "))
}

// ============================================================================
// BuildRmPlan
// ============================================================================

// BuildRmPlan constructs a structured execution plan for ext rm.
// It shows what will happen without actually executing the removal.
func BuildRmPlan(pgVer int, names []string, yes bool) *output.Plan {
	if Catalog == nil {
		return &output.Plan{
			Command:  buildRmCommand(names),
			Actions:  []output.Action{},
			Affects:  []output.Resource{},
			Expected: "error: extension catalog not initialized",
			Risks:    []string{"Extension catalog not loaded, cannot resolve packages"},
		}
	}
	if len(names) == 0 {
		return &output.Plan{
			Command:  buildRmCommand(names),
			Actions:  []output.Action{},
			Affects:  []output.Resource{},
			Expected: "error: no extensions specified",
			Risks:    []string{"No extension names were provided"},
		}
	}

	if pgVer == 0 {
		pgVer = PostgresLatestMajorVersion
	}

	resolved, notFound := resolvePlanExtensions(pgVer, names, false)

	return buildRmPlanFromState(names, resolved, notFound, pgVer, yes)
}

// buildRmPlanFromState constructs a remove plan from given state.
// This is separated for easier testing.
func buildRmPlanFromState(names []string, resolved []resolvedExt, notFound []string, pgVer int, yes bool) *output.Plan {
	if len(resolved) == 0 {
		return &output.Plan{
			Command:  buildRmCommand(names),
			Actions:  []output.Action{},
			Affects:  []output.Resource{},
			Expected: "No packages to remove",
			Risks:    nil,
		}
	}

	actions := buildRmActions(resolved, notFound, pgVer, yes)
	affects := buildRmAffects(resolved)
	expected := buildRmExpected(resolved)
	risks := buildRmRisks(resolved, notFound)

	return &output.Plan{
		Command:  buildRmCommand(names),
		Actions:  actions,
		Affects:  affects,
		Expected: expected,
		Risks:    risks,
	}
}

func buildRmActions(resolved []resolvedExt, notFound []string, pgVer int, yes bool) []output.Action {
	actions := []output.Action{}
	step := 1

	if len(resolved) > 0 {
		var resolveDetails []string
		for _, r := range resolved {
			pkgStr := strings.Join(r.packages, " ")
			resolveDetails = append(resolveDetails, fmt.Sprintf("%s -> %s", r.name, pkgStr))
		}
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Resolve extension names: %s", strings.Join(resolveDetails, ", ")),
		})
		step++

		var allPkgs []string
		for _, r := range resolved {
			allPkgs = append(allPkgs, r.packages...)
		}
		pkgMgr := PackageManagerCmd()
		yesArg := ""
		if yes {
			yesArg = " -y"
		}
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Execute: sudo %s remove%s %s", pkgMgr, yesArg, strings.Join(allPkgs, " ")),
		})
		step++
	}

	if len(notFound) > 0 {
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Skip (not found): %s", strings.Join(notFound, ", ")),
		})
	}

	return actions
}

func buildRmAffects(resolved []resolvedExt) []output.Resource {
	affects := []output.Resource{}

	for _, r := range resolved {
		for _, pkg := range r.packages {
			detail := ""
			if r.ext != nil {
				detail = r.ext.EnDesc
			}
			affects = append(affects, output.Resource{
				Type:   "package",
				Name:   pkg,
				Impact: "remove",
				Detail: detail,
			})
		}
	}

	return affects
}

func buildRmExpected(resolved []resolvedExt) string {
	var names []string
	for _, r := range resolved {
		names = append(names, r.name)
	}
	return fmt.Sprintf("Extensions removed: %s", strings.Join(names, ", "))
}

func buildRmRisks(resolved []resolvedExt, notFound []string) []string {
	var risks []string

	// Check for dependents (RequiredBy / Dependency reverse map)
	for _, r := range resolved {
		if r.ext != nil {
			dependents := r.ext.DependsOn()
			if len(dependents) > 0 {
				risks = append(risks, fmt.Sprintf("Dependent extensions: %s (require %s)", strings.Join(dependents, ", "), r.name))
			}
			// Also check RequireBy field directly
			if len(r.ext.RequireBy) > 0 {
				// Only add if different from DependsOn result
				if len(dependents) == 0 {
					risks = append(risks, fmt.Sprintf("Required by: %s", strings.Join(r.ext.RequireBy, ", ")))
				}
			}
		}
	}

	// General removal risks
	if len(resolved) > 0 {
		risks = append(risks, "Database objects using these extensions will become inaccessible")
		risks = append(risks, "Applications relying on these extensions may fail")
	}

	if len(notFound) > 0 {
		risks = append(risks, fmt.Sprintf("Extensions not found in catalog: %s", strings.Join(notFound, ", ")))
	}

	return risks
}

func buildRmCommand(names []string) string {
	return fmt.Sprintf("pig ext rm %s", strings.Join(names, " "))
}
