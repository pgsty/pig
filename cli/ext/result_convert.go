/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package ext

import "fmt"

/********************
 * Conversion Methods
 ********************/

// ToSummary converts an Extension to ExtensionSummary
func (e *Extension) ToSummary(pgVer int) *ExtensionSummary {
	if e == nil {
		return nil
	}
	status := "not_avail"
	if Postgres != nil && Postgres.ExtensionMap != nil && Postgres.ExtensionMap[e.Name] != nil {
		status = "installed"
	} else if e.Available(pgVer) {
		status = "available"
	}

	return &ExtensionSummary{
		Name:        e.Name,
		Pkg:         e.Pkg,
		Version:     e.Version,
		Category:    e.Category,
		License:     e.License,
		Repo:        e.RepoName(),
		Status:      status,
		PackageName: e.PackageName(pgVer),
		Description: e.EnDesc,
		PgVer:       e.PgVer,
	}
}

// ToInfoData converts an Extension to ExtensionInfoData
func (e *Extension) ToInfoData() *ExtensionInfoData {
	if e == nil {
		return nil
	}

	// Get dependents safely (DependsOn requires Catalog to be initialized)
	var requiredBy []string
	if Catalog != nil && Catalog.ExtNameMap != nil {
		requiredBy = e.DependsOn()
	}

	info := &ExtensionInfoData{
		Name:        e.Name,
		Pkg:         e.Pkg,
		LeadExt:     e.LeadExt,
		Category:    e.Category,
		License:     e.License,
		Language:    e.Lang,
		Version:     e.Version,
		URL:         e.URL,
		Source:      e.Source,
		Description: e.EnDesc,
		ZhDesc:      e.ZhDesc,
		Properties: &ExtensionProperties{
			HasBin:      e.HasBin,
			HasLib:      e.HasLib,
			NeedLoad:    e.NeedLoad,
			NeedDDL:     e.NeedDDL,
			Relocatable: e.Relocatable,
			Trusted:     e.Trusted,
		},
		Requires:   e.Requires,
		RequiredBy: requiredBy,
		SeeAlso:    e.SeeAlso,
		PgVer:      e.PgVer,
		Schemas:    e.Schemas,
		Comment:    e.Comment,
	}

	// RPM package info
	if e.RpmRepo != "" {
		info.RpmPackage = &PackageInfo{
			Package:    e.RpmPkg,
			Repository: e.RpmRepo,
			Version:    e.RpmVer,
			PgVer:      e.RpmPg,
			Deps:       e.RpmDeps,
		}
	}

	// DEB package info
	if e.DebRepo != "" {
		info.DebPackage = &PackageInfo{
			Package:    e.DebPkg,
			Repository: e.DebRepo,
			Version:    e.DebVer,
			PgVer:      e.DebPg,
			Deps:       e.DebDeps,
		}
	}

	// Operations
	info.Operations = &ExtensionOperations{
		Install: fmt.Sprintf("pig ext add %s", e.Pkg),
		Build:   e.GetBuildCommand(),
	}

	if e.NeedLoad {
		libName := e.GetExtraString("lib")
		if libName == "" {
			libName = e.Name
		}
		info.Operations.Config = fmt.Sprintf("shared_preload_libraries = '%s'", libName)
	}

	if e.NeedDDL {
		if len(e.Requires) > 0 {
			info.Operations.Create = fmt.Sprintf("CREATE EXTENSION %s CASCADE;", e.Name)
		} else {
			info.Operations.Create = fmt.Sprintf("CREATE EXTENSION %s;", e.Name)
		}
	}

	return info
}
