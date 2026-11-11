package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"slices"
	"strconv"
	"strings"
)

type protectedRemovalRule struct {
	pattern string
	aliases string
	hint    string
}

const postgresRemovalAliases = "postgresql pgsql pgsql-mini pgsql-core pgsql-full pgsql-main"

// These are package boundaries, not extension metadata. Keep authorization in
// code: a catalog refresh must never grant a new alias permission to remove a
// kernel. Independent PL packages and ordinary shared extension packages are
// intentionally absent. Rationale: https://pig.pgsty.com/design/protected-package-removal/
var protectedRemovalRules = []protectedRemovalRule{
	{"postgresql-$v", postgresRemovalAliases + " pgsql-server", "pg$v-server"},
	{"postgresql$v-server", postgresRemovalAliases + " pgsql-server", "pg$v-server"},
	{"postgresql$v-contrib", postgresRemovalAliases + " pgsql-server pgsql-contrib", "pg$v-contrib"},
	{"postgresql$v-libs", postgresRemovalAliases + " pgsql-server", "pg$v-server"},
	{"postgresql$v", postgresRemovalAliases + " pgsql-client", "pg$v-client"},
	{"pgedge-$v", "pgedge", "pgedge -v $v"},
	{"orioledb-$v", "orioledb-server", "orioledb-server -v $v"},
	{"ivorysql-$v", "ivorysql ivorysqldb ivorysql5 ivorysql-5", "ivorysql -v $v"},
	{"ivorysql-$v-contrib", "ivorysql ivorysqldb", "ivorysql -v $v"},
	{"babelfish-$v", "babelfish babelfishpg", "babelfish -v $v"},
	{"openhalodb-$v", "openhalo openhalodb", "openhalo -v $v"},
	{"polardb-$v", "polardb polar", "polardb -v $v"},
	{"agensgraph-$v", "agensgraph agens", "agensgraph -v $v"},
	{"pgtde-$v", "pgtde", "pgtde (PG 18 only)"},
	{"pgtde-$v-contrib", "pgtde", "pgtde (PG 18 only)"},
	{"percona-postgresql$v", "percona-core percona-main", "percona-core (PG 18 only)"},
	{"percona-postgresql$v-server", "percona-core percona-main", "percona-core (PG 18 only)"},
	{"percona-postgresql$v-contrib", "percona-core percona-main", "percona-core (PG 18 only)"},
	{"percona-postgresql-$v", "percona-core percona-main", "percona-core (PG 18 only)"},
	{"cloudberry", "cloudberry", "cloudberry"},
	{"open-source-greenplum-db-7", "greenplum", "greenplum"},
}

// checkRemovalSafety checks the complete package list of EVERY operand, not
// just its unique contribution to the command. One failure blocks the request.
func checkRemovalSafety(pgVer int, resolution *PackageResolveResult) []*FailedExtItem {
	var failed []*FailedExtItem
	for _, request := range resolution.requests {
		alias, target := removalAliasTarget(request.alias, pgVer)
		if request.source == packageExplicitAlias && alias == "pgsql-contrib" && config.OSType == config.DistroDEB {
			failed = append(failed, protectedRemovalFailure(request.input, "",
				fmt.Sprintf("%q has no separate contrib package on Debian/Ubuntu; use the explicit alias pg%d-server only to remove the server package", request.input, target)))
			continue
		}
		for _, pkg := range request.packages {
			if reason := protectedRemovalReason(request, pkg, alias, target); reason != "" {
				failed = append(failed, protectedRemovalFailure(request.input, pkg, reason))
			}
		}
	}
	return failed
}

func protectedRemovalReason(request packageRequest, pkg, alias string, target int) string {
	for _, rule := range protectedRemovalRules {
		major, matched := protectedPackageMajor(pkg, rule.pattern)
		if !matched {
			continue
		}
		if request.source == packageExplicitAlias && (major == 0 || major == target) && slices.Contains(strings.Fields(rule.aliases), alias) {
			return ""
		}
		hint := strings.ReplaceAll(rule.hint, "$v", strconv.Itoa(major))
		return fmt.Sprintf("%q resolves to protected package %q; use the explicit alias %s to remove that package group", request.input, pkg, hint)
	}
	return ""
}

func protectedRemovalFailure(input, pkg, reason string) *FailedExtItem {
	return &FailedExtItem{Name: input, Package: pkg, Error: reason, Code: output.CodeExtensionProtectedPackage}
}

func removalSafetyMessage(failed []*FailedExtItem) string {
	reasons := make([]string, 0, len(failed))
	for _, item := range failed {
		reasons = append(reasons, item.Error)
	}
	return "removal blocked; no packages removed: " + strings.Join(reasons, "; ")
}

// Exact template matching keeps postgresql18-pltcl and postgresql-18-postgis-3
// outside the server boundary, while catching a catalog entry for any major.
func protectedPackageMajor(pkg, pattern string) (int, bool) {
	prefix, suffix, versioned := strings.Cut(pattern, "$v")
	if !versioned {
		return 0, pkg == pattern
	}
	if !strings.HasPrefix(pkg, prefix) || !strings.HasSuffix(pkg, suffix) || len(pkg) <= len(prefix)+len(suffix) {
		return 0, false
	}
	value := pkg[len(prefix) : len(pkg)-len(suffix)]
	major, err := strconv.Atoi(value)
	return major, err == nil && major > 0 && strconv.Itoa(major) == value
}

func removalAliasTarget(alias string, pgVer int) (string, int) {
	for _, major := range PostgresInstallableMajorVersions {
		prefix := fmt.Sprintf("pg%d", major)
		if alias == prefix || strings.HasPrefix(alias, prefix+"-") {
			return "pgsql" + strings.TrimPrefix(alias, prefix), major
		}
	}
	switch alias {
	case "pgtde", "percona-core", "percona-main", "ivorysql5", "ivorysql-5":
		return alias, 18 // These existing aliases explicitly pin PG 18.
	default:
		return alias, pgVer
	}
}
