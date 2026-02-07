/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"sort"
	"strings"
)

/********************
 * Text() Methods for DTOs
 * Implement output.Texter interface for text-mode rendering
 ********************/

// Text returns a human-readable tabulated extension list.
// Matches output quality of TabulteCommon/TabulteVersion.
func (d *ExtensionListData) Text() string {
	if d == nil {
		return ""
	}

	var sb strings.Builder

	// Build table using output.RenderTable
	if d.PgVersion > 0 {
		// Version-aware mode (like TabulteVersion)
		headers := []string{"Name", "Status", "Version", "Cate", "Flags", "License", "Repo", "PGVer", "Package", "Description"}
		if ShowPkg {
			headers[0] = "Pkg"
		}
		rows := make([][]string, 0, len(d.Extensions))
		for _, ext := range d.Extensions {
			if ext == nil {
				continue
			}
			desc := ext.Description
			if len(desc) > 64 {
				desc = desc[:64]
			}
			firstCol := ext.Name
			if ShowPkg {
				firstCol = ext.Pkg
			}
			pkgStr := ext.PackageName
			if strings.Contains(pkgStr, "$v") {
				pkgStr = fmt.Sprintf("[%s]", pkgStr)
			}
			pgVer := ""
			if len(ext.PgVer) > 0 {
				pgVer = CompactVersion(ext.PgVer)
			}
			rows = append(rows, []string{firstCol, ext.Status, ext.Version, ext.Category, flagsFromSummary(ext), ext.License, ext.Repo, pgVer, pkgStr, desc})
		}
		sb.WriteString(output.RenderTable(headers, rows))
	} else {
		// Common mode (like TabulteCommon): no status column, show RPM/DEB repo instead
		headers := []string{"Name", "Version", "Cate", "Flags", "License", "RPM", "DEB", "PG Ver", "Description"}
		if ShowPkg {
			headers[0] = "Pkg"
		}
		rows := make([][]string, 0, len(d.Extensions))
		for _, ext := range d.Extensions {
			if ext == nil {
				continue
			}
			desc := ext.Description
			if len(desc) > 64 {
				desc = desc[:64] + "..."
			}
			firstCol := ext.Name
			if ShowPkg {
				firstCol = ext.Pkg
			}
			// For common mode, we need RPM/DEB repo info - use Repo as fallback
			rpmRepo := ""
			debRepo := ""
			pgVer := ""
			if len(ext.PgVer) > 0 {
				pgVer = CompactVersion(ext.PgVer)
			}
			// Look up the extension in catalog for RPM/DEB repo info
			if Catalog != nil {
				if e, ok := Catalog.ExtNameMap[ext.Name]; ok {
					rpmRepo = e.RpmRepo
					debRepo = e.DebRepo
				}
			}
			rows = append(rows, []string{firstCol, ext.Version, ext.Category, flagsFromSummary(ext), ext.License, rpmRepo, debRepo, pgVer, desc})
		}
		sb.WriteString(output.RenderTable(headers, rows))
	}

	sb.WriteString(fmt.Sprintf("\n(%d Rows)", len(d.Extensions)))
	return sb.String()
}

// flagsFromSummary reconstructs flag string from summary.
// Since ExtensionSummary doesn't carry flag details, look up from catalog.
func flagsFromSummary(s *ExtensionSummary) string {
	if s == nil {
		return ""
	}
	if Catalog != nil {
		if e, ok := Catalog.ExtNameMap[s.Name]; ok {
			return e.GetFlag()
		}
	}
	return ""
}

// Text returns a human-readable formatted extension info.
// Matches output quality of Extension.FormatInfo().
func (d *ExtensionInfoData) Text() string {
	if d == nil {
		return ""
	}
	// Delegate to FormatInfo on the original Extension if available in catalog
	if Catalog != nil {
		if e, ok := Catalog.ExtNameMap[d.Name]; ok {
			return e.FormatInfo()
		}
	}
	// Fallback: build a simplified text representation from DTO data
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Extension: %s (%s)\n", d.Name, d.Pkg))
	sb.WriteString(fmt.Sprintf("  Category: %s  License: %s  Language: %s\n", d.Category, d.License, d.Language))
	sb.WriteString(fmt.Sprintf("  Version: %s  PG: %s\n", d.Version, strings.Join(d.PgVer, ", ")))
	sb.WriteString(fmt.Sprintf("  Description: %s\n", d.Description))
	if d.URL != "" {
		sb.WriteString(fmt.Sprintf("  URL: %s\n", d.URL))
	}
	if d.Operations != nil {
		sb.WriteString(fmt.Sprintf("  Install: %s\n", d.Operations.Install))
		if d.Operations.Config != "" {
			sb.WriteString(fmt.Sprintf("  Config: %s\n", d.Operations.Config))
		}
		if d.Operations.Create != "" {
			sb.WriteString(fmt.Sprintf("  Create: %s\n", d.Operations.Create))
		}
	}
	return sb.String()
}

// Text returns a human-readable extension status summary.
// Matches output quality of ExtensionStatus().
func (d *ExtensionStatusData) Text() string {
	if d == nil {
		return ""
	}

	var sb strings.Builder

	// PostgreSQL info summary (like PostgresInstallSummary)
	if d.PgInfo != nil {
		sb.WriteString(fmt.Sprintf("PostgreSQL %d: %s\n", d.PgInfo.MajorVersion, d.PgInfo.Version))
		sb.WriteString(fmt.Sprintf("  Binary: %s\n", d.PgInfo.BinDir))
		sb.WriteString(fmt.Sprintf("  Extension: %s\n", d.PgInfo.ExtensionDir))
	}

	// Extension summary
	if d.Summary != nil {
		nonContribCnt := 0
		parts := make([]string, 0)
		for repo, count := range d.Summary.ByRepo {
			if repo != "CONTRIB" {
				nonContribCnt += count
				parts = append(parts, fmt.Sprintf("%s %d", repo, count))
			}
		}
		contribCnt := d.Summary.ByRepo["CONTRIB"]
		sort.Strings(parts)
		sb.WriteString(fmt.Sprintf("\nExtension Stat: %d Installed (%s) + %d CONTRIB = %d Total\n",
			nonContribCnt, strings.Join(parts, ", "), contribCnt, d.Summary.TotalInstalled))
	}

	// Extension table
	if len(d.Extensions) > 0 {
		headers := []string{"Name", "Version", "Cate", "Flags", "License", "Repo", "Package", "Description"}
		if ShowPkg {
			headers[0] = "Pkg"
		}
		rows := make([][]string, 0, len(d.Extensions))
		for _, ext := range d.Extensions {
			if ext == nil {
				continue
			}
			desc := ext.Description
			if len(desc) > 64 {
				desc = desc[:64]
			}
			firstCol := ext.Name
			if ShowPkg {
				firstCol = ext.Pkg
			}
			rows = append(rows, []string{firstCol, ext.Version, ext.Category, flagsFromSummary(ext), ext.License, ext.Repo, ext.PackageName, desc})
		}
		sb.WriteString("\n")
		sb.WriteString(output.RenderTable(headers, rows))
		sb.WriteString(fmt.Sprintf("\n(%d Rows)", len(d.Extensions)))
	}

	if len(d.NotFound) > 0 {
		sb.WriteString(fmt.Sprintf("\nNot found in catalog: %s", strings.Join(d.NotFound, ", ")))
	}

	return sb.String()
}

// Text returns a human-readable scan result.
// Matches output quality of PostgresInstallSummary + ExtensionInstallSummary.
func (d *ScanResultData) Text() string {
	if d == nil {
		return ""
	}

	var sb strings.Builder

	// PostgreSQL info
	if d.PgInfo != nil {
		sb.WriteString(fmt.Sprintf("PostgreSQL %d: %s\n", d.PgInfo.MajorVersion, d.PgInfo.Version))
		sb.WriteString(fmt.Sprintf("  Binary: %s\n", d.PgInfo.BinDir))
		sb.WriteString(fmt.Sprintf("  Extension: %s\n", d.PgInfo.ExtensionDir))
	}

	// Extension table
	if len(d.Extensions) > 0 {
		headers := []string{"Name", "Version", "Description", "Meta"}
		rows := make([][]string, 0, len(d.Extensions))
		for _, ext := range d.Extensions {
			if ext == nil {
				continue
			}
			desc := ext.Description
			if len(desc) > 64 {
				desc = desc[:64] + "..."
			}
			meta := ""
			if len(ext.ControlMeta) > 0 {
				metaParts := make([]string, 0, len(ext.ControlMeta))
				for k, v := range ext.ControlMeta {
					metaParts = append(metaParts, fmt.Sprintf("%s=%s", k, v))
				}
				meta = strings.Join(metaParts, " ")
			}
			if len(ext.Libraries) > 0 {
				if meta != "" {
					meta += " "
				}
				meta += "lib=" + strings.Join(ext.Libraries, ", ")
			}
			rows = append(rows, []string{ext.Name, ext.Version, desc, meta})
		}
		sb.WriteString("\n")
		sb.WriteString(output.RenderTable(headers, rows))
	}

	// Unmatched/encoding/builtin libs
	if len(d.EncodingLibs) > 0 {
		sort.Strings(d.EncodingLibs)
		sb.WriteString(fmt.Sprintf("\nEncoding Libs: %s", strings.Join(d.EncodingLibs, ", ")))
	}
	if len(d.BuiltInLibs) > 0 {
		sort.Strings(d.BuiltInLibs)
		sb.WriteString(fmt.Sprintf("\nBuilt-in Libs: %s", strings.Join(d.BuiltInLibs, ", ")))
	}
	if len(d.UnmatchedLibs) > 0 {
		sort.Strings(d.UnmatchedLibs)
		sb.WriteString(fmt.Sprintf("\nUnmatched Shared Libraries: %s", strings.Join(d.UnmatchedLibs, ", ")))
	}

	return sb.String()
}

// Text returns a human-readable availability matrix.
// Matches output quality of PrintAvailability/PrintGlobalAvailability.
func (d *ExtensionAvailData) Text() string {
	if d == nil {
		return ""
	}

	// Single extension mode
	if d.Extension != "" {
		return d.textSingleExtension()
	}

	// Global availability mode
	return d.textGlobalAvailability()
}

func (d *ExtensionAvailData) textSingleExtension() string {
	if d == nil {
		return ""
	}

	// Try to use the rich matrix display from the Extension object
	if Catalog != nil {
		if e, ok := Catalog.ExtNameMap[d.Extension]; ok {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("\n%s (%s) - %s\n", e.Name, e.Pkg, e.EnDesc))

			leadExt := getLeadExtension(e)
			if leadExt != e {
				sb.WriteString(fmt.Sprintf("(Matrix data from lead extension: %s)\n", leadExt.Name))
			}

			matrix := leadExt.GetPkgMatrix()
			if len(matrix) == 0 {
				sb.WriteString("No availability matrix data available")
				return sb.String()
			}

			// Info line
			if ver := matrix.LatestVersion(); ver != "" {
				sb.WriteString("Latest: " + ver + " | ")
			}
			sb.WriteString(matrix.Summary())
			if pgVers := leadExt.GetPGVersions(); len(pgVers) > 0 {
				pgStrs := make([]string, len(pgVers))
				for i, pg := range pgVers {
					pgStrs[i] = fmt.Sprintf("PG%d", pg)
				}
				sb.WriteString(", " + strings.Join(pgStrs, ", "))
			}
			sb.WriteString(fmt.Sprintf("\nDetails: https://pgext.cloud/e/%s  %s\n\n", e.Name, colorLegend()))
			sb.WriteString(matrix.TabulateAvailability())
			return sb.String()
		}
	}

	// Fallback: simple text from DTO data
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Extension: %s\n", d.Extension))
	if d.LatestVer != "" {
		sb.WriteString(fmt.Sprintf("Latest: %s | %s\n", d.LatestVer, d.Summary))
	}
	if len(d.Matrix) > 0 {
		headers := []string{"OS", "Arch", "PG", "State", "Version", "Org"}
		rows := make([][]string, 0, len(d.Matrix))
		for _, entry := range d.Matrix {
			if entry == nil {
				continue
			}
			rows = append(rows, []string{entry.OS, entry.Arch, fmt.Sprintf("%d", entry.PG), entry.State, entry.Version, entry.Org})
		}
		sb.WriteString(output.RenderTable(headers, rows))
	}
	return sb.String()
}

func (d *ExtensionAvailData) textGlobalAvailability() string {
	if d == nil {
		return ""
	}

	// Try to use the rich global display
	if Catalog != nil && len(Catalog.Extensions) > 0 {
		var sb strings.Builder
		osCode := d.OSCode
		arch := d.Arch
		if osCode == "" {
			osCode = config.OSCode
		}
		if arch == "" {
			arch = config.OSArch
		}

		if !validOSCodes[osCode] {
			sb.WriteString(fmt.Sprintf("\nNote: Current OS '%s' is not a supported Linux distribution.\n", osCode))
			sb.WriteString("Supported OS: el8, el9, el10, d12, d13, u22, u24\n")
			sb.WriteString("Showing matrix for el9.x86_64 as example:\n")
			osCode, arch = "el9", "amd64"
		}

		var packages []*Extension
		for _, ext := range Catalog.Extensions {
			if ext.Contrib || !ext.Lead {
				continue
			}
			packages = append(packages, ext)
		}

		osName := osFullName(osCode, arch)
		sb.WriteString(fmt.Sprintf("\nExtension Availability on %s : https://pgext.cloud/os/%s\n", osName, osName))
		sb.WriteString(fmt.Sprintf("Showing %d packages with %d extensions  %s\n\n", len(packages), len(Catalog.Extensions), colorLegend()))

		sort.Slice(packages, func(i, j int) bool {
			return packages[i].ID < packages[j].ID
		})

		sb.WriteString(tabulateGlobalMatrix(packages, osCode, arch, []int{18, 17, 16, 15, 14, 13}))
		return sb.String()
	}

	// Fallback: simple text from DTO data
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Packages available on %s.%s: %d\n", d.OSCode, d.Arch, d.PackageCount))
	if len(d.Packages) > 0 {
		headers := []string{"Pkg", "Versions"}
		rows := make([][]string, 0, len(d.Packages))
		for _, pkg := range d.Packages {
			if pkg == nil {
				continue
			}
			verParts := make([]string, 0, len(pkg.Versions))
			for pg, ver := range pkg.Versions {
				verParts = append(verParts, fmt.Sprintf("%s:%s", pg, ver))
			}
			sort.Strings(verParts)
			rows = append(rows, []string{pkg.Pkg, strings.Join(verParts, " ")})
		}
		sb.WriteString(output.RenderTable(headers, rows))
	}
	return sb.String()
}

// Text returns a human-readable summary of extension add operation.
func (d *ExtensionAddData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	if len(d.Installed) > 0 {
		sb.WriteString(fmt.Sprintf("Installed %d package(s) for PostgreSQL %d:\n", len(d.Installed), d.PgVersion))
		for _, item := range d.Installed {
			if item == nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("  - %s (%s)\n", item.Name, item.Package))
		}
	}
	if len(d.Failed) > 0 {
		sb.WriteString(fmt.Sprintf("Failed %d package(s):\n", len(d.Failed)))
		for _, item := range d.Failed {
			if item == nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", item.Name, item.Error))
		}
	}
	if d.DurationMs > 0 {
		sb.WriteString(fmt.Sprintf("Duration: %dms\n", d.DurationMs))
	}
	return sb.String()
}

// Text returns a human-readable summary of extension remove operation.
func (d *ExtensionRmData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	if len(d.Removed) > 0 {
		sb.WriteString(fmt.Sprintf("Removed %d package(s) for PostgreSQL %d:\n", len(d.Removed), d.PgVersion))
		for _, pkg := range d.Removed {
			sb.WriteString(fmt.Sprintf("  - %s\n", pkg))
		}
	}
	if len(d.Failed) > 0 {
		sb.WriteString(fmt.Sprintf("Failed %d package(s):\n", len(d.Failed)))
		for _, item := range d.Failed {
			if item == nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", item.Name, item.Error))
		}
	}
	if d.DurationMs > 0 {
		sb.WriteString(fmt.Sprintf("Duration: %dms\n", d.DurationMs))
	}
	return sb.String()
}

// Text returns a human-readable summary of extension update operation.
func (d *ExtensionUpdateData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	if len(d.Updated) > 0 {
		sb.WriteString(fmt.Sprintf("Updated %d package(s) for PostgreSQL %d:\n", len(d.Updated), d.PgVersion))
		for _, pkg := range d.Updated {
			sb.WriteString(fmt.Sprintf("  - %s\n", pkg))
		}
	}
	if len(d.Failed) > 0 {
		sb.WriteString(fmt.Sprintf("Failed %d package(s):\n", len(d.Failed)))
		for _, item := range d.Failed {
			if item == nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", item.Name, item.Error))
		}
	}
	if d.DurationMs > 0 {
		sb.WriteString(fmt.Sprintf("Duration: %dms\n", d.DurationMs))
	}
	return sb.String()
}

// Text returns a human-readable summary of extension import operation.
func (d *ImportResultData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Import to %s for PostgreSQL %d:\n", d.RepoDir, d.PgVersion))
	if len(d.Downloaded) > 0 {
		sb.WriteString(fmt.Sprintf("  Downloaded %d package(s):\n", len(d.Downloaded)))
		for _, pkg := range d.Downloaded {
			sb.WriteString(fmt.Sprintf("    - %s\n", pkg))
		}
	}
	if len(d.Failed) > 0 {
		sb.WriteString(fmt.Sprintf("  Failed %d package(s):\n", len(d.Failed)))
		for _, pkg := range d.Failed {
			sb.WriteString(fmt.Sprintf("    - %s\n", pkg))
		}
	}
	if d.DurationMs > 0 {
		sb.WriteString(fmt.Sprintf("Duration: %dms\n", d.DurationMs))
	}
	return sb.String()
}

// Text returns a human-readable summary of link operation.
func (d *LinkResultData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	if d.Action == "unlink" {
		sb.WriteString(fmt.Sprintf("Unlinked PostgreSQL from %s\n", d.SymlinkPath))
	} else {
		sb.WriteString(fmt.Sprintf("Linked %s -> %s\n", d.SymlinkPath, d.PgHome))
	}
	sb.WriteString(fmt.Sprintf("Profile: %s\n", d.ProfilePath))
	if d.ActivatedCmd != "" {
		sb.WriteString(fmt.Sprintf("Activate: %s\n", d.ActivatedCmd))
	}
	return sb.String()
}

// Text returns a human-readable summary of catalog reload operation.
func (d *ReloadResultData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Reloaded catalog from %s\n", d.SourceURL))
	sb.WriteString(fmt.Sprintf("  Extensions: %d\n", d.ExtensionCount))
	sb.WriteString(fmt.Sprintf("  Saved to: %s\n", d.CatalogPath))
	if d.DurationMs > 0 {
		sb.WriteString(fmt.Sprintf("  Duration: %dms\n", d.DurationMs))
	}
	return sb.String()
}
