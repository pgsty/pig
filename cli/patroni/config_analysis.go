package patroni

import (
	"sort"
	"strings"
)

// pgRestartParams is the union of PostgreSQL 14-19 core parameters known to
// use postmaster context. It is intentionally static: pt config must not query
// pg_settings at runtime. Extra entries are acceptable because the UX rule is
// to prefer warning about a restart over missing one.
var pgRestartParams = map[string]struct{}{
	"archive_mode":                        {},
	"autovacuum_freeze_max_age":           {},
	"autovacuum_multixact_freeze_max_age": {},
	"autovacuum_worker_slots":             {},
	"bonjour":                             {},
	"bonjour_name":                        {},
	"cluster_name":                        {},
	"commit_timestamp_buffers":            {},
	"data_directory":                      {},
	"data_sync_retry":                     {},
	"dynamic_shared_memory_type":          {},
	"event_source":                        {},
	"external_pid_file":                   {},
	"hba_file":                            {},
	"hot_standby":                         {},
	"huge_page_size":                      {},
	"huge_pages":                          {},
	"ident_file":                          {},
	"io_max_combine_limit":                {},
	"io_max_concurrency":                  {},
	"io_method":                           {},
	"jit_provider":                        {},
	"listen_addresses":                    {},
	"logging_collector":                   {},
	"max_active_replication_origins":      {},
	"max_connections":                     {},
	"max_files_per_process":               {},
	"max_locks_per_transaction":           {},
	"max_logical_replication_workers":     {},
	"max_notify_queue_pages":              {},
	"max_pred_locks_per_transaction":      {},
	"max_prepared_transactions":           {},
	"max_replication_slots":               {},
	"max_wal_senders":                     {},
	"max_worker_processes":                {},
	"min_dynamic_shared_memory":           {},
	"multixact_member_buffers":            {},
	"multixact_offset_buffers":            {},
	"notify_buffers":                      {},
	"old_snapshot_threshold":              {},
	"port":                                {},
	"recovery_target":                     {},
	"recovery_target_action":              {},
	"recovery_target_inclusive":           {},
	"recovery_target_lsn":                 {},
	"recovery_target_name":                {},
	"recovery_target_time":                {},
	"recovery_target_timeline":            {},
	"recovery_target_xid":                 {},
	"reserved_connections":                {},
	"serializable_buffers":                {},
	"shared_buffers":                      {},
	"shared_memory_type":                  {},
	"shared_preload_libraries":            {},
	"subtransaction_buffers":              {},
	"superuser_reserved_connections":      {},
	"track_activity_query_size":           {},
	"track_commit_timestamp":              {},
	"transaction_buffers":                 {},
	"unix_socket_directories":             {},
	"unix_socket_group":                   {},
	"unix_socket_permissions":             {},
	"wal_buffers":                         {},
	"wal_decode_buffer_size":              {},
	"wal_level":                           {},
	"wal_log_hints":                       {},
}

// PGConfigAnalysis describes static impact classification for pt config pg.
type PGConfigAnalysis struct {
	RestartParams   []string
	RequiresRestart bool
}

// AnalyzePGConfigPairs classifies PostgreSQL config pairs with a static
// restart-parameter set. It never connects to PostgreSQL.
func AnalyzePGConfigPairs(kvPairs []string) PGConfigAnalysis {
	seen := make(map[string]struct{})
	for _, pair := range kvPairs {
		name := normalizePGConfigName(pair)
		if name == "" {
			continue
		}
		if IsPGRestartParam(name) {
			seen[name] = struct{}{}
		}
	}

	restartParams := make([]string, 0, len(seen))
	for name := range seen {
		restartParams = append(restartParams, name)
	}
	sort.Strings(restartParams)
	return PGConfigAnalysis{
		RestartParams:   restartParams,
		RequiresRestart: len(restartParams) > 0,
	}
}

// IsPGRestartParam reports whether name is in the static postmaster-context
// union. Names are matched case-insensitively.
func IsPGRestartParam(name string) bool {
	_, ok := pgRestartParams[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func normalizePGConfigName(pair string) string {
	name, _, _ := strings.Cut(pair, "=")
	return strings.ToLower(strings.TrimSpace(name))
}
