/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Shared constants used across multiple packages.
*/
package utils

// MinDataDirFileCount is the minimum number of files expected in an initialized
// PostgreSQL data directory. A typical initialized PGDATA contains at least:
// base/, global/, pg_wal/, pg_xact/, pg_multixact/, pg_subtrans/,
// PG_VERSION, postgresql.conf, pg_hba.conf, pg_ident.conf, etc.
// This constant is used for sanity checks when detecting if a directory is initialized.
const MinDataDirFileCount = 10
