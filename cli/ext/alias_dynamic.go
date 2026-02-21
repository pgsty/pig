package ext

import (
	"fmt"
	"pig/internal/config"
	"slices"
	"strconv"
	"strings"
	"sync"
)

var (
	categoryAliasSet = map[string]struct{}{
		"time":  {},
		"gis":   {},
		"rag":   {},
		"fts":   {},
		"olap":  {},
		"feat":  {},
		"lang":  {},
		"type":  {},
		"util":  {},
		"func":  {},
		"admin": {},
		"stat":  {},
		"sec":   {},
		"fdw":   {},
		"sim":   {},
		"etl":   {},
	}

	categoryAliasCache sync.Map // key -> []string
)

type categoryAliasSpec struct {
	category string
	targetPG int
	isPgsql  bool
}

// resolveAliasPattern resolves both static aliases and dynamic category aliases.
// Returns:
// - pattern: space-separated package pattern list
// - matched: true if alias is recognized (static or dynamic category)
// - noPackage: true if alias is recognized but resolved package list is empty
func resolveAliasPattern(pgVer int, alias string) (pattern string, matched bool, noPackage bool) {
	if Catalog == nil {
		return "", false, false
	}

	if pgPkg, ok := Catalog.AliasMap[alias]; ok {
		if strings.TrimSpace(pgPkg) == "" {
			return "", true, true
		}
		return pgPkg, true, false
	}

	pkgs, catMatched := resolveCategoryAliasPackages(alias, pgVer)
	if !catMatched {
		return "", false, false
	}
	if len(pkgs) == 0 {
		return "", true, true
	}
	return strings.Join(pkgs, " "), true, false
}

func resolveCategoryAliasPackages(alias string, targetPgVer int) ([]string, bool) {
	spec, ok := parseCategoryAlias(alias, targetPgVer)
	if !ok || Catalog == nil {
		return nil, false
	}

	matrixOS, arch, allowMetadataFallback := resolveCategoryAliasMatrixTarget()
	cacheKey := fmt.Sprintf("%p|%s|%s|%s|%s|%d|%s|%t|%t", Catalog, config.OSType, config.OSCode, matrixOS, arch, spec.targetPG, spec.category, allowMetadataFallback, spec.isPgsql)
	if v, ok := categoryAliasCache.Load(cacheKey); ok {
		if cached, ok := v.([]string); ok {
			return cached, true
		}
	}

	pkgList := buildCategoryPackageList(spec, matrixOS, arch, allowMetadataFallback)
	categoryAliasCache.Store(cacheKey, pkgList)
	return pkgList, true
}

func parseCategoryAlias(alias string, targetPgVer int) (spec categoryAliasSpec, ok bool) {
	if strings.HasPrefix(alias, "pgsql-") {
		category := strings.TrimPrefix(alias, "pgsql-")
		if _, exists := categoryAliasSet[category]; !exists {
			return categoryAliasSpec{}, false
		}
		if targetPgVer == 0 {
			targetPgVer = PostgresLatestMajorVersion
		}
		return categoryAliasSpec{
			category: category,
			targetPG: targetPgVer,
			isPgsql:  true,
		}, true
	}

	if !strings.HasPrefix(alias, "pg") {
		return categoryAliasSpec{}, false
	}

	parts := strings.SplitN(strings.TrimPrefix(alias, "pg"), "-", 2)
	if len(parts) != 2 {
		return categoryAliasSpec{}, false
	}

	ver, err := strconv.Atoi(parts[0])
	if err != nil {
		return categoryAliasSpec{}, false
	}
	if !slices.Contains(PostgresActiveMajorVersions, ver) {
		return categoryAliasSpec{}, false
	}

	category := parts[1]
	if _, exists := categoryAliasSet[category]; !exists {
		return categoryAliasSpec{}, false
	}

	return categoryAliasSpec{
		category: category,
		targetPG: ver,
		isPgsql:  false,
	}, true
}

func resolveCategoryAliasMatrixTarget() (osCode, arch string, allowMetadataFallback bool) {
	arch = normalizeMatrixArch(config.OSArch)

	switch config.OSType {
	case config.DistroEL:
		switch config.OSCode {
		case "el8", "el9", "el10":
			return config.OSCode, arch, false
		default:
			return "el10", arch, true
		}
	case config.DistroDEB:
		switch config.OSCode {
		case "d12", "d13", "u22", "u24":
			return config.OSCode, arch, false
		default:
			return "d13", arch, true
		}
	default:
		return config.OSCode, arch, false
	}
}

func normalizeMatrixArch(arch string) string {
	switch arch {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return "amd64"
	}
}

func buildCategoryPackageList(spec categoryAliasSpec, matrixOS, matrixArch string, allowMetadataFallback bool) []string {
	if Catalog == nil {
		return nil
	}

	pkgs := make([]string, 0, 16)
	seen := make(map[string]struct{})
	selectPG := spec.targetPG
	if spec.isPgsql {
		selectPG = PostgresLatestMajorVersion
	}

	for _, ext := range Catalog.Extensions {
		if ext == nil || !ext.Lead || ext.Contrib {
			continue
		}
		if strings.ToLower(ext.Category) != spec.category {
			continue
		}

		if !isCategoryExtensionVisible(ext, selectPG, matrixOS, matrixArch, allowMetadataFallback) {
			continue
		}

		pkgName := ext.PackageName(selectPG)
		if pkgName == "" {
			continue
		}

		if !spec.isPgsql {
			pkgName = applyCategoryPackageSpecialCase(ext, pkgName, spec.targetPG)
		}

		for _, pkg := range ProcessPkgName(pkgName, selectPG) {
			if spec.isPgsql {
				pkg = rewriteLatestCategoryPkgToTarget(pkg, spec.targetPG)
			}
			if _, ok := seen[pkg]; ok {
				continue
			}
			seen[pkg] = struct{}{}
			pkgs = append(pkgs, pkg)
		}
	}

	return pkgs
}

func isCategoryExtensionVisible(ext *Extension, pgVer int, matrixOS, matrixArch string, allowMetadataFallback bool) bool {
	if ext == nil {
		return false
	}

	matrix := ext.GetPkgMatrix()
	if matrix != nil {
		if entry := matrix.Get(matrixOS, matrixArch, pgVer); entry != nil {
			return entry.State == PkgAvail && !entry.Hide
		}
		if !allowMetadataFallback {
			return false
		}
	} else if !allowMetadataFallback {
		return false
	}

	// Fallback for future/unsupported OS codes:
	// use repository and pg-version metadata when matrix row is missing.
	verStr := strconv.Itoa(pgVer)
	switch config.OSType {
	case config.DistroEL:
		return slices.Contains(ext.RpmPg, verStr) && repoIsPGDG(ext.RpmRepo)
	case config.DistroDEB:
		return slices.Contains(ext.DebPg, verStr) && repoIsPGDG(ext.DebRepo)
	default:
		return false
	}
}

func repoIsPGDG(repo string) bool {
	return strings.EqualFold(strings.TrimSpace(repo), "PGDG")
}

func applyCategoryPackageSpecialCase(ext *Extension, pkgName string, pgVer int) string {
	if config.OSType == config.DistroEL && (ext.Pkg == "pgaudit" || ext.Name == "pgaudit") {
		switch pgVer {
		case 15:
			return "pgaudit17_15"
		case 14:
			return "pgaudit16_14"
		case 13:
			return "pgaudit15_13"
		}
	}
	return pkgName
}

func rewriteLatestCategoryPkgToTarget(pkg string, targetPG int) string {
	if targetPG == PostgresLatestMajorVersion {
		return pkg
	}

	latest := strconv.Itoa(PostgresLatestMajorVersion)
	target := strconv.Itoa(targetPG)

	switch config.OSType {
	case config.DistroEL:
		return strings.ReplaceAll(pkg, "_"+latest, "_"+target)
	case config.DistroDEB:
		return strings.ReplaceAll(pkg, "-"+latest, "-"+target)
	default:
		return strings.ReplaceAll(pkg, latest, target)
	}
}
