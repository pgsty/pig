/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Plan building for ext add/rm commands.
*/
package ext

import (
	"fmt"
	"pig/internal/output"
	"strings"
)

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
		pgVer = PostgresLatestMajorVersion()
	}

	resolution := ResolveExtensionPackages(pgVer, names, true)
	resolved, notFound := resolution.resolved, resolution.unresolved
	return buildAddPlanFromState(names, resolved, notFound, pgVer, yes)
}

// buildAddPlanFromState constructs an add plan from given state.
// This is separated for easier testing.
func buildAddPlanFromState(names []string, resolved []resolvedExt, notFound []string, pgVer int, yes bool) *output.Plan {
	actions := buildAddActions(resolved, notFound, pgVer, yes)
	affects := buildAddAffects(resolved, pgVer)
	expected := buildAddExpected(resolved)
	risks := buildAddRisks(resolved, notFound)

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
			if len(r.packages) == 0 {
				continue
			}
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
		command := buildPackageManagerCommand(pkgOpInstall, yes, allPkgs)
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Execute: sudo %s", strings.Join(command, " ")),
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

func buildAddExpected(resolved []resolvedExt) string {
	if len(resolved) > 0 {
		var names []string
		for _, r := range resolved {
			names = append(names, r.name)
		}
		return fmt.Sprintf("Extensions installed: %s", strings.Join(names, ", "))
	}
	return "No extensions to install"
}

func buildAddRisks(resolved []resolvedExt, notFound []string) []string {
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
		pgVer = PostgresLatestMajorVersion()
	}

	resolution := ResolveExtensionPackages(pgVer, names, false)
	resolved, notFound := resolution.resolved, resolution.unresolved

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
			if len(r.packages) == 0 {
				continue
			}
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
		command := buildPackageManagerCommand(pkgOpRemove, yes, allPkgs)
		actions = append(actions, output.Action{
			Step:        step,
			Description: fmt.Sprintf("Execute: sudo %s", strings.Join(command, " ")),
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
