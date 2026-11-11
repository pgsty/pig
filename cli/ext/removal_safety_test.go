package ext

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
)

// These tests change catalog/config globals and PATH; do not run them in parallel.
func removalSafetyPlatforms(t *testing.T, test func(*testing.T)) {
	t.Helper()
	for _, platform := range []struct{ distro, version, code string }{
		{config.DistroDEB, "12", "d12"},
		{config.DistroEL, "9", "el9"},
	} {
		t.Run(platform.distro, func(t *testing.T) {
			t.Cleanup(withResolveAliasTestEnv(t, platform.distro, platform.version, platform.code, "amd64", loadEmbeddedCatalogForTest(t)))
			clearCategoryAliasCache()
			t.Cleanup(clearCategoryAliasCache)
			test(t)
		})
	}
}

// PATH contains ONLY inert recorders. Even a regression calling sudo cannot
// forward to a host package manager. Scripts use /bin/sh and shell builtins.
func removalSafetyRecorder(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := `#!/bin/sh
{
  printf '%s' "${0##*/}"
  printf '\t%s' "$@"
  printf '\n'
} >> "$PIG_TEST_REMOVAL_CALLS"
case "${0##*/}" in sudo) exit 99;; esac
`
	for _, name := range []string{"apt-get", "dnf", "yum", "sudo"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("PIG_NO_SUDO", "1")
	t.Setenv("PIG_TEST_REMOVAL_CALLS", calls)
	return calls
}

func removalSafetyCalls(t *testing.T, path string, want []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	expected := ""
	if len(want) > 0 {
		expected = strings.Join(want, "\t") + "\n"
	}
	if string(data) != expected {
		t.Fatalf("native calls = %q, want %q", data, expected)
	}
}

func removalSafetyBlocked(t *testing.T, major int, names []string, yes bool, fragments ...string) {
	t.Helper()
	calls := removalSafetyRecorder(t)
	plan, err := BuildRmPlan(major, names, yes)
	removalSafetyCalls(t, calls, nil)
	result := RmExtensions(major, names, yes)
	removalSafetyCalls(t, calls, nil)
	if result == nil || result.Success || result.Code != 100102 || result.ExitCode() != 2 {
		t.Fatalf("runtime result = %#v, want protected-package error 100102 / exit 2", result)
	}
	if plan == nil || err == nil || len(plan.Actions) != 0 {
		t.Fatalf("blocked plan = %#v, error = %v; want error and zero actions", plan, err)
	}
	if result.Message != err.Error() {
		t.Errorf("runtime reason %q differs from plan reason %q", result.Message, err)
	}
	var failedDetails []string
	for _, check := range plan.Preconditions {
		if check.Status == "failed" {
			failedDetails = append(failedDetails, check.Detail)
		}
	}
	if len(failedDetails) == 0 {
		t.Fatal("blocked plan has no failed precondition")
	}
	for _, fragment := range fragments {
		if !strings.Contains(result.Message, fragment) || !strings.Contains(strings.Join(failedDetails, "\n"), fragment) {
			t.Errorf("runtime/plan must explain %q: runtime=%q, preconditions=%v", fragment, result.Message, failedDetails)
		}
	}
	data, ok := result.Data.(*ExtensionRmData)
	if !ok || len(data.Removed) != 0 || !slices.Equal(data.Requested, names) {
		t.Fatalf("blocked removal data = %#v; want original requests and zero removed packages", result.Data)
	}
}

func removalSafetyAllowed(t *testing.T, major int, names, want []string, yes bool) *output.Plan {
	t.Helper()
	calls := removalSafetyRecorder(t)
	plan, err := BuildRmPlan(major, names, yes)
	removalSafetyCalls(t, calls, nil)
	if err != nil || plan == nil {
		t.Fatalf("allowed plan = %#v, error = %v", plan, err)
	}
	for _, check := range plan.Preconditions {
		if check.Status == "failed" {
			t.Fatalf("allowed plan failed precondition: %+v", check)
		}
	}
	var affected []string
	for _, resource := range plan.Affects {
		if resource.Type == "package" {
			affected = append(affected, resource.Name)
		}
	}
	if !slices.Equal(affected, want) {
		t.Errorf("plan packages = %v, want %v", affected, want)
	}
	command := []string{PackageManagerCmd(), "remove"}
	if yes {
		command = append(command, "-y")
	}
	command = append(command, want...)
	execution := "Execute: sudo " + strings.Join(command, " ")
	if !slices.ContainsFunc(plan.Actions, func(action output.Action) bool { return action.Description == execution }) {
		t.Errorf("plan actions = %+v, want %q", plan.Actions, execution)
	}
	result := RmExtensions(major, names, yes)
	removalSafetyCalls(t, calls, command)
	if result == nil || !result.Success || result.ExitCode() != 0 {
		t.Fatalf("allowed removal failed: %#v", result)
	}
	data, ok := result.Data.(*ExtensionRmData)
	if !ok || !slices.Equal(data.Packages, want) || !slices.Equal(data.Removed, want) || !slices.Equal(data.Requested, names) {
		t.Fatalf("removal data = %#v, want packages/removed=%v and requests=%v", result.Data, want, names)
	}
	return plan
}

func TestRemovalSafetyEmbeddedCoreAndIndependentPackages(t *testing.T) {
	removalSafetyPlatforms(t, func(t *testing.T) {
		for _, major := range []int{14, 15, 16, 17, 18, 19} {
			t.Run(fmt.Sprintf("pg%d", major), func(t *testing.T) {
				for _, name := range []string{"pg_trgm", "earthdistance", "tablefunc", "plpgsql"} {
					t.Run(name, func(t *testing.T) {
						pkg := fmt.Sprintf("postgresql-%d", major)
						hint := fmt.Sprintf("pg%d-server", major)
						if config.OSType == config.DistroEL {
							pkg, hint = fmt.Sprintf("postgresql%d-contrib", major), fmt.Sprintf("pg%d-contrib", major)
							if name == "plpgsql" {
								pkg, hint = fmt.Sprintf("postgresql%d-server", major), fmt.Sprintf("pg%d-server", major)
							}
						}
						removalSafetyBlocked(t, major, []string{name}, true, name, pkg, hint)
					})
				}
				t.Run("independent_shared_packages", func(t *testing.T) {
					names := []string{"plpython3u", "pltcl", "pltclu", "plperl", "plperlu", "vector", "pgvector", "postgis", "postgis_topology"}
					patterns := []string{"postgresql-plpython3-%d", "postgresql-pltcl-%d", "postgresql-plperl-%d", "postgresql-%d-pgvector", "postgresql-%d-postgis-3"}
					if config.OSType == config.DistroEL {
						patterns = []string{"postgresql%d-plpython3", "postgresql%d-pltcl", "postgresql%d-plperl", "pgvector_%d", "postgis37_%d"}
					}
					var want []string
					for _, pattern := range patterns {
						want = append(want, fmt.Sprintf(pattern, major))
					}
					plan := removalSafetyAllowed(t, major, names, want, false)
					for _, shared := range []string{"pltclu", "plperlu", "postgis_raster"} {
						if !slices.ContainsFunc(plan.Affects, func(resource output.Resource) bool { return strings.Contains(resource.Detail, shared) }) {
							t.Errorf("plan does not describe shared extension %s", shared)
						}
					}
				})
			})
		}
	})
}

func TestRemovalSafetyEmbeddedForkExtensions(t *testing.T) {
	removalSafetyPlatforms(t, func(t *testing.T) {
		for _, test := range []struct {
			name, pkg, alias string
			major            int
		}{
			{"spock", "pgedge-18", "pgedge", 18},
			{"snowflake", "pgedge-17", "pgedge", 17},
			{"lolor", "pgedge-15", "pgedge", 15},
			{"ivorysql_ora", "ivorysql-18", "ivorysql", 18},
			{"babelfishpg_tsql", "babelfish-17", "babelfish", 17},
			{"aux_mysql", "openhalodb-14", "openhalo", 14},
			{"orioledb", "orioledb-16", "orioledb-server", 16},
			{"pg_tde", "pgtde-18", "pgtde", 18},
		} {
			t.Run(test.name, func(t *testing.T) {
				removalSafetyBlocked(t, test.major, []string{test.name}, true, test.name, test.pkg, test.alias)
			})
		}
	})
}

func TestRemovalSafetyExplicitAliases(t *testing.T) {
	removalSafetyPlatforms(t, func(t *testing.T) {
		// Existing alias composition is covered by catalog tests. Here its complete
		// resolved package list must survive both the removal guard and the plan.
		for _, aliases := range []struct {
			names string
			major int
		}{
			{"pg18 pg18-server pg18-core pg18-mini pg18-full pg18-main pg19-server", 14},
			{"pgsql-server pgsql-client pg18-client postgresql pgedge ivorysql ivorysqldb orioledb-server", 18},
			{"babelfish babelfishpg", 17},
			{"openhalo openhalodb", 14},
			{"agensgraph agens", 18},
			{"polardb polar", 15},
			{"pgtde percona-core percona-main cloudberry greenplum", 17},
		} {
			for _, alias := range strings.Fields(aliases.names) {
				if alias == "greenplum" && config.OSType == config.DistroDEB {
					continue // The existing Greenplum alias is EL-only.
				}
				t.Run(alias, func(t *testing.T) {
					resolved := ResolveExtensionPackages(aliases.major, []string{alias}, false)
					if len(resolved.Packages) == 0 || len(resolved.NotFound)+len(resolved.NoPackage) != 0 {
						t.Fatalf("registered alias %s did not resolve: %+v", alias, resolved)
					}
					if (strings.HasPrefix(alias, "pg18") || strings.HasPrefix(alias, "pg19")) && !strings.HasSuffix(alias, "-client") {
						major := 18
						if alias == "pg19-server" {
							major = 19
						}
						server := fmt.Sprintf("postgresql-%d", major)
						if config.OSType == config.DistroEL {
							server = fmt.Sprintf("postgresql%d-server", major)
						}
						if !slices.Contains(resolved.Packages, server) {
							t.Fatalf("alias major must override -v 14: %s = %v", alias, resolved.Packages)
						}
					}
					removalSafetyAllowed(t, aliases.major, []string{alias}, resolved.Packages, true)
				})
			}
		}
		for _, alias := range []string{"pg18-contrib", "pgsql-contrib"} {
			t.Run(alias, func(t *testing.T) {
				if config.OSType == config.DistroDEB {
					removalSafetyBlocked(t, 18, []string{"vector", alias}, true, alias, "no separate contrib", "pg18-server")
					return
				}
				removalSafetyAllowed(t, 18, []string{alias}, []string{"postgresql18-contrib"}, true)
			})
		}
	})
}

func TestRemovalSafetyMixedRequestsKeepEveryOperand(t *testing.T) {
	removalSafetyPlatforms(t, func(t *testing.T) {
		for _, pair := range [][2]string{
			{"vector", "pg_trgm"},
			{"pg18-server", "pg_trgm"},
			{"pg18-server", "plpgsql"},
			{"pgedge", "spock"},
			{"orioledb-server", "orioledb"},
			{"pg_trgm", "earthdistance"},
			{"pg_trgm", "pg_trgm"},
		} {
			for _, reverse := range []bool{false, true} {
				names := []string{pair[0], pair[1]}
				if reverse {
					slices.Reverse(names)
				}
				t.Run(strings.Join(names, "+"), func(t *testing.T) {
					pkg := Catalog.ExtNameMap[pair[1]].PackageName(18)
					removalSafetyBlocked(t, 18, names, true, pair[1], pkg)
				})
			}
		}
		t.Run("confirmation_is_not_authorization", func(t *testing.T) {
			removalSafetyBlocked(t, 18, []string{"pg_trgm"}, false, "pg_trgm")
		})
		t.Run("alias_does_not_authorize_other_major", func(t *testing.T) {
			removalSafetyBlocked(t, 17, []string{"pg18-server", "plpgsql"}, true, "plpgsql", Catalog.ExtNameMap["plpgsql"].PackageName(17), "pg17-server")
		})
	})
}

func TestRemovalSafetyIndirectPackageRequests(t *testing.T) {
	removalSafetyPlatforms(t, func(t *testing.T) {
		// Real fork entries are hidden from category expansion. Make just this
		// copied entry visible and give it an unregistered normalized package name.
		fork := *Catalog.ExtNameMap["spock"]
		fork.Pkg = "kernel_bundle"
		fork.Extra = map[string]interface{}{"matrix": []interface{}{"d12i:18:A:f:1:P:18.6", "el9i:18:A:f:1:P:18.6"}}
		t.Cleanup(withCategoryAliasTestEnv(t, config.OSType, config.OSCode, config.OSArch, []*Extension{&fork}))
		for _, test := range []struct {
			name  string
			major int
		}{
			{"kernel_bundle", 18},
			{"pg18-etl", 17},
			{"pgsql-etl", 18},
		} {
			t.Run(test.name, func(t *testing.T) {
				removalSafetyBlocked(t, test.major, []string{test.name}, true, test.name, "pgedge-18", "pgedge -v 18")
			})
		}
	})
}

func TestRemovalSafetyAliasAuthorizationHasExactScope(t *testing.T) {
	removalSafetyPlatforms(t, func(t *testing.T) {
		server17, server18 := "postgresql-17", "postgresql-18"
		if config.OSType == config.DistroEL {
			server17, server18 = "postgresql17-server", "postgresql18-server"
		}
		for _, test := range []struct{ name, alias, pkg string }{
			{"wrong_major", "pg18-server", server17},
			{"wrong_family", "pg18-server", "pgedge-18"},
			{"client_cannot_authorize_server", "pg18-client", server18},
			{"unregistered_alias", "custom-server", server18},
		} {
			t.Run(test.name, func(t *testing.T) {
				old := OSAliasOverride
				OSAliasOverride = map[string]map[string]string{config.OSCode: {test.alias: test.pkg}}
				t.Cleanup(func() { OSAliasOverride = old })
				removalSafetyBlocked(t, 17, []string{test.alias}, true, test.alias, test.pkg)
			})
		}
	})
}

func TestRemovalSafetyYumUsesSameBoundary(t *testing.T) {
	t.Cleanup(withResolveAliasTestEnv(t, config.DistroEL, "7", "el7", "amd64", loadEmbeddedCatalogForTest(t)))
	if got := PackageManagerCmd(); got != "yum" {
		t.Fatalf("EL7 package manager = %q, want yum", got)
	}
	t.Run("blocked", func(t *testing.T) {
		removalSafetyBlocked(t, 14, []string{"vector", "plpgsql"}, true, "postgresql14-server")
	})
	t.Run("allowed", func(t *testing.T) {
		removalSafetyAllowed(t, 14, []string{"pltcl", "pltclu"}, []string{"postgresql14-pltcl"}, true)
	})
}

func TestRemovalSafetyDoesNotProtectInstallOrUpdate(t *testing.T) {
	removalSafetyPlatforms(t, func(t *testing.T) {
		for _, operation := range []string{"install", "update"} {
			t.Run(operation, func(t *testing.T) {
				calls := removalSafetyRecorder(t)
				names := []string{"pg_trgm", "plpgsql", "spock", "orioledb"}
				want := []string{"postgresql-18", "pgedge-18", "orioledb-18"}
				if config.OSType == config.DistroEL {
					want = []string{"postgresql18-contrib", "postgresql18-server", "pgedge-18", "orioledb-18"}
				}
				command := []string{PackageManagerCmd(), operation}
				var result *output.Result
				if operation == "install" {
					plan := BuildAddPlan(18, names, true)
					if plan == nil || len(plan.Actions) == 0 {
						t.Fatalf("install plan lost protected packages: %#v", plan)
					}
					removalSafetyCalls(t, calls, nil)
					result = AddExtensions(18, names, true)
				} else {
					if config.OSType == config.DistroDEB {
						command = []string{"apt-get", "install", "--only-upgrade"}
					}
					result = UpgradeExtensions(18, names, true)
				}
				command = append(append(command, "-y"), want...)
				removalSafetyCalls(t, calls, command)
				if result == nil || !result.Success {
					t.Fatalf("%s must retain existing behavior: %#v", operation, result)
				}
			})
		}
	})
}
