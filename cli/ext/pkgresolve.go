package ext

import (
	"fmt"
	"pig/internal/config"
	"strconv"
	"strings"
)

// PackageResolveResult contains resolved package names and resolution metadata.
type PackageResolveResult struct {
	Packages     []string
	PackageOwner map[string]string
	NotFound     []string
	NoPackage    []string
}

type packageResolveOptions struct {
	EnableTranslation bool
	FallbackToRaw     bool
	ParseVersion      bool
}

// PackageManagerCmd returns the package manager binary for current OS.
func PackageManagerCmd() string {
	switch config.OSType {
	case config.DistroEL:
		if config.OSVersion == "8" || config.OSVersion == "9" || config.OSVersion == "10" {
			return "dnf"
		}
		return "yum"
	case config.DistroDEB:
		return "apt-get"
	default:
		return ""
	}
}

// ProcessPkgName expands package patterns and replaces $v with target PostgreSQL version.
func ProcessPkgName(pkgName string, pgVer int) []string {
	if pkgName == "" {
		return []string{}
	}
	parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(pkgName), ",", " "), " ")
	var pkgNames []string
	pkgNameSet := make(map[string]struct{})
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		partStr := strings.ReplaceAll(part, "$v", strconv.Itoa(pgVer))
		if _, exists := pkgNameSet[partStr]; !exists {
			pkgNames = append(pkgNames, partStr)
			pkgNameSet[partStr] = struct{}{}
		}
	}
	return pkgNames
}

// ResolveInstallPackages resolves package names for install command.
// If noTranslation is true, raw package names are used directly.
func ResolveInstallPackages(pgVer int, names []string, noTranslation bool) *PackageResolveResult {
	return resolvePackageRequests(pgVer, names, packageResolveOptions{
		EnableTranslation: !noTranslation,
		FallbackToRaw:     true,
		ParseVersion:      true,
	})
}

// ResolveExtensionPackages resolves package names for ext add/rm/update operations.
// parseVersionSpec should be true only for add operation.
func ResolveExtensionPackages(pgVer int, names []string, parseVersionSpec bool) *PackageResolveResult {
	return resolvePackageRequests(pgVer, names, packageResolveOptions{
		EnableTranslation: true,
		FallbackToRaw:     false,
		ParseVersion:      parseVersionSpec,
	})
}

func resolvePackageRequests(pgVer int, names []string, opts packageResolveOptions) *PackageResolveResult {
	res := &PackageResolveResult{
		Packages:     make([]string, 0, len(names)),
		PackageOwner: make(map[string]string),
	}
	seenPackages := make(map[string]struct{})

	if opts.EnableTranslation && Catalog != nil {
		Catalog.LoadAliasMap(config.OSType)
	}

	for _, raw := range names {
		name, version := splitNameVersion(raw, opts.ParseVersion)
		pkgPattern, owner, reason := resolvePackagePattern(pgVer, raw, name, opts)

		switch reason {
		case "not_found":
			res.NotFound = append(res.NotFound, raw)
			continue
		case "no_package":
			res.NoPackage = append(res.NoPackage, raw)
			continue
		}

		for _, pkg := range ProcessPkgName(pkgPattern, pgVer) {
			resolvedPkg := applyPackageVersion(pkg, version)
			if _, exists := seenPackages[resolvedPkg]; exists {
				continue
			}
			seenPackages[resolvedPkg] = struct{}{}
			res.Packages = append(res.Packages, resolvedPkg)
			res.PackageOwner[resolvedPkg] = owner
		}
	}

	return res
}

func splitNameVersion(raw string, parse bool) (name string, version string) {
	name = raw
	if !parse {
		return name, ""
	}
	parts := strings.Split(raw, "=")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return raw, ""
}

func resolvePackagePattern(pgVer int, rawName, baseName string, opts packageResolveOptions) (pkgPattern string, owner string, reason string) {
	if opts.EnableTranslation && Catalog != nil {
		if ext, ok := Catalog.ExtNameMap[baseName]; ok {
			pkgName := ext.PackageName(pgVer)
			if pkgName == "" {
				return "", "", "no_package"
			}
			return pkgName, ext.Name, ""
		}
		if ext, ok := Catalog.ExtPkgMap[baseName]; ok {
			pkgName := ext.PackageName(pgVer)
			if pkgName == "" {
				return "", "", "no_package"
			}
			return pkgName, ext.Name, ""
		}
		if pgPkg, ok := Catalog.AliasMap[baseName]; ok {
			return pgPkg, rawName, ""
		}
		if !opts.FallbackToRaw {
			return "", "", "not_found"
		}
	}

	if opts.FallbackToRaw {
		return baseName, baseName, ""
	}
	return "", "", "not_found"
}

func applyPackageVersion(pkg, version string) string {
	if version == "" {
		return pkg
	}
	switch config.OSType {
	case config.DistroEL:
		return fmt.Sprintf("%s-%s", pkg, version)
	case config.DistroDEB:
		return fmt.Sprintf("%s=%s*", pkg, version)
	default:
		return pkg
	}
}
