package ext

import (
	"fmt"
	"strings"
)

// FormatInfo returns a formatted string of extension information
func (e *Extension) FormatInfo() string {
	var sb strings.Builder

	// Top border
	sb.WriteString("╭──────────────────────────────────────────────────────────────────────────────────────────────╮\n")

	// Name
	sb.WriteString(fmt.Sprintf("│ %-92s │\n", e.Name))
	sb.WriteString("├──────────────────────────────────────────────────────────────────────────────────────────────┤\n")

	// Description
	sb.WriteString(fmt.Sprintf("│ %-92s │\n", truncate(e.EnDesc, 92)))
	sb.WriteString("├──────────────┬───────────────────────────────────────────────────────────────────────────────┤\n")

	// Basic info (removed State line)
	sb.WriteString(fmt.Sprintf("│ Extension    │ %-77s │\n", e.Name))
	sb.WriteString(fmt.Sprintf("│ Package      │ %-77s │\n", e.Pkg))
	sb.WriteString(fmt.Sprintf("│ Leading Ext  │ %-77s │\n", e.LeadExt))
	sb.WriteString(fmt.Sprintf("│ Category     │ %-77s │\n", e.Category))
	sb.WriteString(fmt.Sprintf("│ License      │ %-77s │\n", e.License))
	sb.WriteString(fmt.Sprintf("│ Language     │ %-77s │\n", e.Lang))
	sb.WriteString(fmt.Sprintf("│ Website      │ %-77s │\n", truncate(e.URL, 80)))
	sb.WriteString(fmt.Sprintf("│ Details      │ %-77s │\n", e.SummaryURL()))
	if e.Source != "" {
		sb.WriteString(fmt.Sprintf("│ Source       │ %-77s │\n", e.Source))
	}

	// Properties section - redesigned with balanced proportions
	sb.WriteString("├──────────────┴───────────────────────────────────────────────────────────────────────────────┤\n")
	sb.WriteString("│ Properties                                                                                   │\n")
	sb.WriteString("├──────────────┬────────────┬─────────────┬───────────┬────────────┬─────────────┬─────────────┤\n")
	sb.WriteString("│  Attributes  │ Has Binary │ Has Library │ Need Load │ Create DDL │ Relocatable │   Trusted   │\n")
	sb.WriteString("├──────────────┼────────────┼─────────────┼───────────┼────────────┼─────────────┼─────────────┤\n")
	sb.WriteString(fmt.Sprintf("│%s│%s│%s│%s│%s│%s│%s│\n",
		center(e.GetFlag(), 14),
		center(boolYesNo(e.HasBin), 12),
		center(boolYesNo(e.HasLib), 13),
		center(boolYesNo(e.NeedLoad), 11),
		center(boolYesNo(e.NeedDDL), 12),
		center(relocStr(e.Relocatable), 13),
		center(trustStr(e.Trusted), 13)))
	sb.WriteString("├──────────────┴────────────┴─────────────┴───────────┴────────────┴─────────────┴─────────────┤\n")

	// Relationship section
	sb.WriteString("│ Relationship                                                                                 │\n")
	sb.WriteString("├──────────────┬───────────────────────────────────────────────────────────────────────────────┤\n")
	sb.WriteString(fmt.Sprintf("│ Requires:    │ %-77s │\n", truncate(join(e.Requires, ", "), 77)))
	sb.WriteString(fmt.Sprintf("│ Required By: │ %-77s │\n", truncate(join(e.DependsOn(), ", "), 77)))
	sb.WriteString(fmt.Sprintf("│ See Also:    │ %-77s │\n", truncate(join(e.SeeAlso, ", "), 77)))
	sb.WriteString("├──────────────┴───────────────────────────────────────────────────────────────────────────────┤\n")

	// EXT Summary section
	sb.WriteString("│ EXT Summary                                                                                  │\n")
	sb.WriteString("├──────────────┬───────────────────────────────────────────────────────────────────────────────┤\n")
	sb.WriteString(fmt.Sprintf("│ Repository   │ %-77s │\n", e.Repo))
	sb.WriteString(fmt.Sprintf("│ Version      │ %-77s │\n", e.Version))
	sb.WriteString(fmt.Sprintf("│ PG Version   │ %-77s │\n", join(e.PgVer, ", ")))
	sb.WriteString(fmt.Sprintf("│ Schemas      │ %-77s │\n", truncate(join(e.Schemas, ", "), 77)))
	sb.WriteString("├──────────────┴───────────────────────────────────────────────────────────────────────────────┤\n")

	// RPM Package section (if available)
	if e.RpmRepo != "" {
		sb.WriteString("│ RPM Package                                                                                  │\n")
		sb.WriteString("├──────────────┬───────────────────────────────────────────────────────────────────────────────┤\n")
		sb.WriteString(fmt.Sprintf("│ Package      │ %-77s │\n", e.RpmPkg))
		sb.WriteString(fmt.Sprintf("│ Repository   │ %-77s │\n", e.RpmRepo))
		sb.WriteString(fmt.Sprintf("│ Version      │ %-77s │\n", e.RpmVer))
		sb.WriteString(fmt.Sprintf("│ PG Version   │ %-77s │\n", join(e.RpmPg, ", ")))
		if len(e.RpmDeps) > 0 {
			sb.WriteString(fmt.Sprintf("│ Dependency   │ %-77s │\n", truncate(join(e.RpmDeps, ", "), 77)))
		}
		sb.WriteString("├──────────────┴───────────────────────────────────────────────────────────────────────────────┤\n")
	}

	// DEB Package section (if available)
	if e.DebRepo != "" {
		sb.WriteString("│ DEB Package                                                                                  │\n")
		sb.WriteString("├──────────────┬───────────────────────────────────────────────────────────────────────────────┤\n")
		sb.WriteString(fmt.Sprintf("│ Package      │ %-77s │\n", e.DebPkg))
		sb.WriteString(fmt.Sprintf("│ Repository   │ %-77s │\n", e.DebRepo))
		sb.WriteString(fmt.Sprintf("│ Version      │ %-77s │\n", e.DebVer))
		sb.WriteString(fmt.Sprintf("│ PG Version   │ %-77s │\n", join(e.DebPg, ", ")))
		if len(e.DebDeps) > 0 {
			sb.WriteString(fmt.Sprintf("│ Dependency   │ %-77s │\n", truncate(join(e.DebDeps, ", "), 77)))
		}
		sb.WriteString("├──────────────┴───────────────────────────────────────────────────────────────────────────────┤\n")
	}

	// Operation section
	sb.WriteString("│ Operation                                                                                    │\n")
	sb.WriteString("├──────────────┬───────────────────────────────────────────────────────────────────────────────┤\n")
	sb.WriteString(fmt.Sprintf("│ INSTALL      │ %-77s │\n", fmt.Sprintf("pig ext add %s", e.Pkg)))

	// CONFIG line - check Extra["lib"] first
	if e.NeedLoad {
		libName := e.GetExtraString("lib")
		if libName == "" {
			libName = e.Name
		}
		sb.WriteString(fmt.Sprintf("│ CONFIG       │ %-77s │\n", fmt.Sprintf("shared_preload_libraries = '%s'", libName)))
	}

	// CREATE line
	if e.NeedDDL {
		createSQL := fmt.Sprintf("CREATE EXTENSION %s;", e.Name)
		if len(e.Requires) > 0 {
			createSQL = fmt.Sprintf("CREATE EXTENSION %s CASCADE;", e.Name)
		}
		sb.WriteString(fmt.Sprintf("│ CREATE       │ %-77s │\n", createSQL))
	}

	// BUILD line - always present
	buildCmd := e.GetBuildCommand()
	sb.WriteString(fmt.Sprintf("│ BUILD        │ %-77s │\n", buildCmd))

	// Comments (only if comment exists)
	if e.Comment != "" {
		sb.WriteString("├──────────────┴───────────────────────────────────────────────────────────────────────────────┤\n")
		sb.WriteString(fmt.Sprintf("│ Comment: %-83s │\n", truncate(e.Comment, 83)))
		sb.WriteString("╰──────────────────────────────────────────────────────────────────────────────────────────────╯")
	} else {
		sb.WriteString("╰──────────────┴───────────────────────────────────────────────────────────────────────────────╯")
	}

	// Bottom border

	return sb.String()
}

// Helper functions

func join(strs []string, sep string) string {
	return strings.Join(strs, sep)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func center(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	padding := width - len(s)
	leftPad := padding / 2
	rightPad := padding - leftPad
	return strings.Repeat(" ", leftPad) + s + strings.Repeat(" ", rightPad)
}

func boolYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func relocStr(s string) string {
	if s == "t" {
		return "Yes"
	}
	if s == "f" {
		return "No"
	}
	return "N/A"
}

func trustStr(s string) string {
	if s == "t" {
		return "Yes"
	}
	if s == "f" {
		return "No"
	}
	return "N/A"
}

// GetExtraString returns a string value from the Extra map
func (e *Extension) GetExtraString(key string) string {
	if e.Extra == nil {
		return ""
	}
	if val, ok := e.Extra[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// GetExtraBool returns a bool value from the Extra map
func (e *Extension) GetExtraBool(key string) bool {
	if e.Extra == nil {
		return false
	}
	if val, ok := e.Extra[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// GetBuildCommand returns the BUILD command with appropriate comment
func (e *Extension) GetBuildCommand() string {
	hasRpm := e.GetExtraBool("rpm")
	hasDeb := e.GetExtraBool("deb")

	if hasRpm && hasDeb {
		return fmt.Sprintf("pig build pkg %s;  # build rpm / deb", e.Pkg)
	} else if hasRpm {
		return fmt.Sprintf("pig build pkg %s;  # build rpm", e.Pkg)
	} else if hasDeb {
		return fmt.Sprintf("pig build pkg %s;  # build deb", e.Pkg)
	}
	return "# no building spec available"
}
