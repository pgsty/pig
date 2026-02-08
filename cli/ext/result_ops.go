/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
)

/********************
 * Result Constructors
 ********************/

// ListExtensions returns a structured Result for the ext list command
func ListExtensions(query string, pgVer int) *output.Result {
	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	results := Catalog.Extensions
	if query != "" {
		results = SearchExtensions(query, Catalog.Extensions)
	}

	// Build extension list with optional ShowPkg filter
	extensions := make([]*ExtensionSummary, 0, len(results))
	for _, ext := range results {
		if ext == nil {
			continue
		}
		// Apply ShowPkg filter if enabled (only show lead extensions)
		if ShowPkg && !ext.Lead {
			continue
		}
		extensions = append(extensions, ext.ToSummary(pgVer))
	}

	data := &ExtensionListData{
		Query:      query,
		PgVersion:  pgVer,
		OSCode:     config.OSCode,
		Arch:       config.OSArch,
		Count:      len(extensions),
		Extensions: extensions,
	}

	message := fmt.Sprintf("Found %d extensions", data.Count)
	if query != "" {
		message = fmt.Sprintf("Found %d extensions matching '%s'", data.Count, query)
	}

	return output.OK(message, data)
}

// GetExtensionInfo returns a structured Result for the ext info command
func GetExtensionInfo(names []string) *output.Result {
	if len(names) == 0 {
		return output.Fail(output.CodeExtensionInvalidArgs, "no extension name provided")
	}

	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	var infos []*ExtensionInfoData
	var notFound []string

	for _, name := range names {
		e, ok := Catalog.ExtNameMap[name]
		if !ok {
			e, ok = Catalog.ExtPkgMap[name]
		}
		if !ok {
			notFound = append(notFound, name)
			continue
		}
		infos = append(infos, e.ToInfoData())
	}

	if len(infos) == 0 {
		return output.Fail(output.CodeExtensionNotFound, fmt.Sprintf("extensions not found: %v", notFound))
	}

	var data interface{}
	var message string
	if len(infos) == 1 {
		data = infos[0]
		message = fmt.Sprintf("Extension: %s", infos[0].Name)
	} else {
		data = infos
		message = fmt.Sprintf("Found %d extensions", len(infos))
	}

	result := output.OK(message, data)
	if len(notFound) > 0 {
		result.Detail = fmt.Sprintf("not found: %v", notFound)
	}
	return result
}

// GetExtStatus returns a structured Result for the ext status command
func GetExtStatus(showContrib bool) *output.Result {
	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	if Postgres == nil {
		return output.Fail(output.CodeExtensionNoPG, "no PostgreSQL specified and no active PostgreSQL found")
	}

	// Collect installed extensions
	var exts []*ExtensionSummary
	var notFound []string
	repoCount := map[string]int{"CONTRIB": 0, "PGDG": 0, "PIGSTY": 0}
	totalInstalled := 0

	for _, installedExt := range Postgres.Extensions {
		if installedExt.Extension == nil {
			continue
		}
		extInfo := Catalog.ExtNameMap[installedExt.Name]
		if extInfo == nil {
			notFound = append(notFound, installedExt.Name)
			continue
		}

		repo := extInfo.RepoName()
		if repo == "" {
			repo = "UNKNOWN"
		}
		if _, ok := repoCount[repo]; !ok {
			repoCount[repo] = 0
		}
		repoCount[repo]++
		totalInstalled++

		if !showContrib && extInfo.Repo == "CONTRIB" {
			continue
		}
		exts = append(exts, extInfo.ToSummary(Postgres.MajorVersion))
	}

	totalShown := len(exts)
	data := &ExtensionStatusData{
		PgInfo: &PostgresInfo{
			Version:      Postgres.Version,
			MajorVersion: Postgres.MajorVersion,
			BinDir:       Postgres.BinPath,
			ExtensionDir: Postgres.ExtPath,
		},
		Summary: &ExtensionSummaryInfo{
			TotalInstalled: totalInstalled,
			ByRepo:         repoCount,
		},
		Extensions: exts,
		NotFound:   notFound,
	}

	message := fmt.Sprintf("PostgreSQL %d: %d extensions installed", Postgres.MajorVersion, totalInstalled)
	if totalShown != totalInstalled {
		message = fmt.Sprintf("PostgreSQL %d: %d extensions installed (%d shown)", Postgres.MajorVersion, totalInstalled, totalShown)
	}
	return output.OK(message, data)
}

// GetExtensionAvailability returns a structured Result for the ext avail command
func GetExtensionAvailability(names []string) *output.Result {
	if Catalog == nil || len(Catalog.Extensions) == 0 {
		return output.Fail(output.CodeExtensionCatalogError, "no extension catalog available")
	}

	osCode := config.OSCode
	arch := config.OSArch

	// No arguments: global availability
	if len(names) == 0 {
		return getGlobalAvailability(osCode, arch)
	}

	// With arguments: per-extension availability
	return getExtensionAvailabilities(names, osCode, arch)
}

func getGlobalAvailability(osCode, arch string) *output.Result {
	// Collect lead packages
	var packages []*PackageAvailability

	for _, ext := range Catalog.Extensions {
		if ext.Contrib || !ext.Lead {
			continue
		}

		matrix := ext.GetPkgMatrix()
		versions := make(map[string]string)

		// Use the centralized PostgreSQL version list
		for _, pg := range PostgresActiveMajorVersions {
			if matrix != nil {
				entry := matrix.Get(osCode, arch, pg)
				if entry != nil && entry.State == PkgAvail {
					versions[fmt.Sprintf("%d", pg)] = entry.Version
				}
			}
		}

		if len(versions) > 0 {
			packages = append(packages, &PackageAvailability{
				Pkg:      ext.Pkg,
				Versions: versions,
			})
		}
	}

	data := &ExtensionAvailData{
		OSCode:       osCode,
		Arch:         arch,
		PackageCount: len(packages),
		Packages:     packages,
	}

	message := fmt.Sprintf("Found %d packages available on %s.%s", len(packages), osCode, arch)
	return output.OK(message, data)
}

// buildExtensionAvailData builds availability data for a single extension
// This is a helper function to avoid code duplication
func buildExtensionAvailData(e *Extension) *ExtensionAvailData {
	if e == nil {
		return nil
	}

	leadExt := getLeadExtension(e)
	matrix := leadExt.GetPkgMatrix()

	entries := make([]*MatrixEntry, 0, len(matrix))
	for _, entry := range matrix {
		if entry != nil {
			entries = append(entries, &MatrixEntry{
				OS:      entry.OS,
				Arch:    entry.Arch,
				PG:      entry.PG,
				State:   string(entry.State),
				Version: entry.Version,
				Org:     string(entry.Org),
			})
		}
	}

	return &ExtensionAvailData{
		Extension: e.Name,
		Matrix:    entries,
		Summary:   matrix.Summary(),
		LatestVer: matrix.LatestVersion(),
	}
}

func getExtensionAvailabilities(names []string, osCode, arch string) *output.Result {
	if len(names) == 1 {
		return getSingleExtensionAvailability(names[0], osCode, arch)
	}

	// Multiple extensions: return array
	var results []*ExtensionAvailData
	var notFound []string

	for _, name := range names {
		e, ok := Catalog.ExtNameMap[name]
		if !ok {
			e, ok = Catalog.ExtPkgMap[name]
		}
		if !ok {
			notFound = append(notFound, name)
			continue
		}

		results = append(results, buildExtensionAvailData(e))
	}

	if len(results) == 0 {
		return output.Fail(output.CodeExtensionNotFound, fmt.Sprintf("extensions not found: %v", notFound))
	}

	message := fmt.Sprintf("Availability for %d extensions", len(results))
	result := output.OK(message, results)
	if len(notFound) > 0 {
		result.Detail = fmt.Sprintf("not found: %v", notFound)
	}
	return result
}

func getSingleExtensionAvailability(name, osCode, arch string) *output.Result {
	e, ok := Catalog.ExtNameMap[name]
	if !ok {
		e, ok = Catalog.ExtPkgMap[name]
	}
	if !ok {
		return output.Fail(output.CodeExtensionNotFound, fmt.Sprintf("extension '%s' not found", name))
	}

	data := buildExtensionAvailData(e)
	message := fmt.Sprintf("Availability for %s: %s", e.Name, data.Summary)
	return output.OK(message, data)
}

// ToScanEntry converts an ExtensionInstall to ScanExtEntry for structured output
func (ei *ExtensionInstall) ToScanEntry() *ScanExtEntry {
	if ei == nil {
		return nil
	}

	entry := &ScanExtEntry{
		Name:        ei.ExtName(),
		ControlName: ei.ControlName,
		Version:     ei.VersionString(),
		Description: ei.Description(),
		InCatalog:   ei.Extension != nil,
		ControlMeta: ei.ControlMeta,
	}

	// Add libraries if present - directly convert from map keys
	if len(ei.Libraries) > 0 {
		libs := make([]string, 0, len(ei.Libraries))
		for lib := range ei.Libraries {
			libs = append(libs, lib)
		}
		entry.Libraries = libs
	}

	return entry
}

// ScanExtensionsResult returns a structured Result for the ext scan command
func ScanExtensionsResult() *output.Result {
	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	if Postgres == nil {
		return output.Fail(output.CodeExtensionNoPG, "no PostgreSQL specified and no active PostgreSQL found")
	}

	// Scan extensions
	if err := Postgres.ScanExtensions(); err != nil {
		return output.Fail(output.CodeExtensionCatalogError, fmt.Sprintf("failed to scan extensions: %v", err))
	}

	// Build extension list
	var extensions []*ScanExtEntry
	for _, installedExt := range Postgres.Extensions {
		if installedExt == nil {
			continue
		}
		extensions = append(extensions, installedExt.ToScanEntry())
	}

	// Collect unmatched, encoding, and built-in libs
	var unmatchedLibs []string
	var encodingLibs []string
	var builtInLibs []string

	for libName, matched := range Postgres.SharedLibs {
		if isEncodingLib(libName) {
			encodingLibs = append(encodingLibs, libName)
			continue
		}
		if isBuiltInLib(libName) {
			builtInLibs = append(builtInLibs, libName)
			continue
		}
		if !matched {
			unmatchedLibs = append(unmatchedLibs, libName)
		}
	}

	data := &ScanResultData{
		PgInfo: &PostgresInfo{
			Version:      Postgres.Version,
			MajorVersion: Postgres.MajorVersion,
			BinDir:       Postgres.BinPath,
			ExtensionDir: Postgres.ExtPath,
		},
		ExtCount:      len(extensions),
		Extensions:    extensions,
		UnmatchedLibs: unmatchedLibs,
		EncodingLibs:  encodingLibs,
		BuiltInLibs:   builtInLibs,
	}

	message := fmt.Sprintf("PostgreSQL %d: scanned %d extensions", Postgres.MajorVersion, len(extensions))
	return output.OK(message, data)
}
