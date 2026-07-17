package cmdb

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

type schemaColumn struct {
	Table    string
	Position int16
	Name     string
	Type     string
	NotNull  bool
	Default  *string
}

var expectedColumns = []schemaColumn{
	{Table: "schema_migration", Position: 1, Name: "version", Type: "bigint", NotNull: true},
	{Table: "schema_migration", Position: 2, Name: "name", Type: "text", NotNull: true},
	{Table: "schema_migration", Position: 3, Name: "checksum", Type: "bytea", NotNull: true},
	{Table: "schema_migration", Position: 4, Name: "tool_version", Type: "text", NotNull: true},
	{Table: "schema_migration", Position: 5, Name: "applied_at", Type: "timestamp with time zone", NotNull: true},
	{Table: "snapshot", Position: 1, Name: "revision", Type: "bigint", NotNull: true},
	{Table: "snapshot", Position: 2, Name: "parent_revision", Type: "bigint"},
	{Table: "snapshot", Position: 3, Name: "merge_base_revision", Type: "bigint", NotNull: true},
	{Table: "snapshot", Position: 4, Name: "semantic_sha256", Type: "bytea", NotNull: true},
	{Table: "snapshot", Position: 5, Name: "payload", Type: "jsonb", NotNull: true},
	{Table: "snapshot", Position: 6, Name: "origin", Type: "text", NotNull: true},
	{Table: "snapshot", Position: 7, Name: "source_kind", Type: "text", NotNull: true},
	{Table: "snapshot", Position: 8, Name: "source_locator", Type: "text"},
	{Table: "snapshot", Position: 9, Name: "source_bytes_sha256", Type: "bytea"},
	{Table: "snapshot", Position: 10, Name: "tool_version", Type: "text", NotNull: true},
	{Table: "snapshot", Position: 11, Name: "created_at", Type: "timestamp with time zone", NotNull: true},
	{Table: "state", Position: 1, Name: "singleton", Type: "boolean", NotNull: true},
	{Table: "state", Position: 2, Name: "head_revision", Type: "bigint"},
	{Table: "state", Position: 3, Name: "updated_at", Type: "timestamp with time zone", NotNull: true},
}

type schemaConstraint struct {
	Table      string
	Name       string
	Kind       string
	Deferrable bool
	Deferred   bool
}

var expectedConstraints = []schemaConstraint{
	{Table: "schema_migration", Name: "schema_migration_checksum_length", Kind: "c"},
	{Table: "schema_migration", Name: "schema_migration_pkey", Kind: "p"},
	{Table: "schema_migration", Name: "schema_migration_version_positive", Kind: "c"},
	{Table: "snapshot", Name: "snapshot_merge_base_revision_fkey", Kind: "f", Deferrable: true, Deferred: true},
	{Table: "snapshot", Name: "snapshot_origin_check", Kind: "c"},
	{Table: "snapshot", Name: "snapshot_parent_revision_fkey", Kind: "f"},
	{Table: "snapshot", Name: "snapshot_pkey", Kind: "p"},
	{Table: "snapshot", Name: "snapshot_revision_positive", Kind: "c"},
	{Table: "snapshot", Name: "snapshot_semantic_sha256_length", Kind: "c"},
	{Table: "snapshot", Name: "snapshot_source_consistency", Kind: "c"},
	{Table: "snapshot", Name: "snapshot_source_kind_check", Kind: "c"},
	{Table: "state", Name: "state_head_revision_fkey", Kind: "f"},
	{Table: "state", Name: "state_pkey", Kind: "p"},
	{Table: "state", Name: "state_singleton_true", Kind: "c"},
}

func validateNativeSchema(ctx context.Context, tx pgx.Tx) error {
	checks := []func(context.Context, pgx.Tx) error{
		validateSchemaRelations,
		validateSchemaColumns,
		validateSchemaConstraints,
		validateSchemaFunction,
		validateSchemaTriggers,
		validateSchemaPrivileges,
	}
	for _, check := range checks {
		if err := check(ctx, tx); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaRelations(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
SELECT c.relname, c.relkind::text, c.relowner = n.nspowner
  FROM pg_catalog.pg_class AS c
  JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
 WHERE n.nspname = 'pigsty_cmdb'
   AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f', 'i')
 ORDER BY c.relname`)
	if err != nil {
		return schemaInvalid("relation catalog")
	}
	defer rows.Close()
	type relation struct{ name, kind string }
	var got []relation
	for rows.Next() {
		var item relation
		var sameOwner bool
		if err := rows.Scan(&item.name, &item.kind, &sameOwner); err != nil || !sameOwner {
			return schemaInvalid("relation ownership")
		}
		got = append(got, item)
	}
	if rows.Err() != nil {
		return schemaInvalid("relation catalog")
	}
	expected := []relation{
		{name: "schema_migration", kind: "r"},
		{name: "schema_migration_pkey", kind: "i"},
		{name: "snapshot", kind: "r"},
		{name: "snapshot_pkey", kind: "i"},
		{name: "state", kind: "r"},
		{name: "state_pkey", kind: "i"},
	}
	if !slices.Equal(got, expected) {
		return schemaInvalid("relation shape")
	}
	return nil
}

func validateSchemaColumns(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
SELECT c.relname, a.attnum, a.attname,
       pg_catalog.format_type(a.atttypid, a.atttypmod),
       a.attnotnull,
       pg_catalog.pg_get_expr(d.adbin, d.adrelid)
  FROM pg_catalog.pg_attribute AS a
  JOIN pg_catalog.pg_class AS c ON c.oid = a.attrelid
  JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
  LEFT JOIN pg_catalog.pg_attrdef AS d
    ON d.adrelid = a.attrelid AND d.adnum = a.attnum
 WHERE n.nspname = 'pigsty_cmdb'
   AND c.relkind = 'r'
   AND a.attnum > 0
   AND NOT a.attisdropped
 ORDER BY c.relname, a.attnum`)
	if err != nil {
		return schemaInvalid("column catalog")
	}
	defer rows.Close()
	var got []schemaColumn
	for rows.Next() {
		var item schemaColumn
		if err := rows.Scan(&item.Table, &item.Position, &item.Name, &item.Type, &item.NotNull, &item.Default); err != nil {
			return schemaInvalid("column catalog")
		}
		got = append(got, item)
	}
	if rows.Err() != nil || !slices.Equal(got, expectedColumns) {
		return schemaInvalid("column shape")
	}
	return nil
}

func validateSchemaConstraints(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
SELECT c.relname, x.conname, x.contype::text, x.condeferrable, x.condeferred
  FROM pg_catalog.pg_constraint AS x
  JOIN pg_catalog.pg_class AS c ON c.oid = x.conrelid
  JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
 WHERE n.nspname = 'pigsty_cmdb'
   AND x.contype <> 'n'
 ORDER BY c.relname, x.conname`)
	if err != nil {
		return schemaInvalid("constraint catalog")
	}
	defer rows.Close()
	var got []schemaConstraint
	for rows.Next() {
		var item schemaConstraint
		if err := rows.Scan(&item.Table, &item.Name, &item.Kind, &item.Deferrable, &item.Deferred); err != nil {
			return schemaInvalid("constraint catalog")
		}
		got = append(got, item)
	}
	if rows.Err() != nil || !slices.Equal(got, expectedConstraints) {
		return schemaInvalid("constraint shape")
	}
	return validateConstraintDefinitions(ctx, tx)
}

func validateConstraintDefinitions(ctx context.Context, tx pgx.Tx) error {
	// Hashing the canonical catalog definitions catches a constraint recreated
	// under an expected name with weaker semantics. The strings are generated
	// by PostgreSQL from the same version-independent v1 DDL.
	rows, err := tx.Query(ctx, `
SELECT x.conname, pg_catalog.pg_get_constraintdef(x.oid, true)
  FROM pg_catalog.pg_constraint AS x
  JOIN pg_catalog.pg_class AS c ON c.oid = x.conrelid
  JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
 WHERE n.nspname = 'pigsty_cmdb'
   AND x.contype <> 'n'
 ORDER BY x.conname`)
	if err != nil {
		return schemaInvalid("constraint definitions")
	}
	defer rows.Close()
	definitions := make(map[string]string, len(expectedConstraints))
	for rows.Next() {
		var name, definition string
		if err := rows.Scan(&name, &definition); err != nil {
			return schemaInvalid("constraint definitions")
		}
		definitions[name] = normalizeCatalogDefinition(definition)
	}
	if rows.Err() != nil {
		return schemaInvalid("constraint definitions")
	}
	for name, expected := range expectedConstraintDefinitions {
		definition, ok := definitions[name]
		if !ok || definition != expected {
			return schemaInvalid("constraint definitions")
		}
	}
	return nil
}

var expectedConstraintDefinitions = map[string]string{
	"schema_migration_version_positive": "check (version > 0)",
	"schema_migration_checksum_length":  "check (octet_length(checksum) = 32)",
	"schema_migration_pkey":             "primary key (version)",
	"snapshot_revision_positive":        "check (revision > 0)",
	"snapshot_semantic_sha256_length":   "check (octet_length(semantic_sha256) = 32)",
	"snapshot_origin_check":             "check (origin = any (array['static_load'::text, 'rollback'::text]))",
	"snapshot_source_kind_check":        "check (source_kind = any (array['static'::text, 'revision'::text]))",
	"snapshot_source_consistency":       "check (origin = 'static_load'::text and source_kind = 'static'::text and source_locator is not null and source_bytes_sha256 is not null and octet_length(source_bytes_sha256) = 32 and merge_base_revision = revision or origin = 'rollback'::text and source_kind = 'revision'::text and source_locator is not null and source_locator ~ '^cmdb-v1:[1-9][0-9]*:sha256:[0-9a-f]{64}$'::text and source_bytes_sha256 is null)",
	"snapshot_parent_revision_fkey":     "foreign key (parent_revision) references pigsty_cmdb.snapshot(revision)",
	"snapshot_merge_base_revision_fkey": "foreign key (merge_base_revision) references pigsty_cmdb.snapshot(revision) deferrable initially deferred",
	"snapshot_pkey":                     "primary key (revision)",
	"state_singleton_true":              "check (singleton)",
	"state_head_revision_fkey":          "foreign key (head_revision) references pigsty_cmdb.snapshot(revision)",
	"state_pkey":                        "primary key (singleton)",
}

func normalizeCatalogDefinition(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func validateSchemaFunction(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
SELECT p.proname, p.pronargs, p.prorettype = 'pg_catalog.trigger'::pg_catalog.regtype,
       l.lanname, p.provolatile::text, p.prosecdef, p.proleakproof,
       p.proowner = n.nspowner, p.prosrc
  FROM pg_catalog.pg_proc AS p
  JOIN pg_catalog.pg_namespace AS n ON n.oid = p.pronamespace
  JOIN pg_catalog.pg_language AS l ON l.oid = p.prolang
 WHERE n.nspname = 'pigsty_cmdb'`)
	if err != nil {
		return schemaInvalid("function catalog")
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		var name, language, volatility, source string
		var arguments int16
		var returnsTrigger, securityDefiner, leakproof, sameOwner bool
		if err := rows.Scan(&name, &arguments, &returnsTrigger, &language, &volatility,
			&securityDefiner, &leakproof, &sameOwner, &source); err != nil {
			return schemaInvalid("function catalog")
		}
		if name != "reject_append_only" || arguments != 0 || !returnsTrigger || language != "plpgsql" ||
			volatility != "v" || securityDefiner || leakproof || !sameOwner ||
			normalizeCatalogDefinition(source) != "begin raise exception using errcode = '55000', message = 'pigsty_cmdb append-only relation'; end;" {
			return schemaInvalid("function shape")
		}
	}
	if rows.Err() != nil || count != 1 {
		return schemaInvalid("function shape")
	}
	return nil
}

func validateSchemaTriggers(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
SELECT c.relname, t.tgname, t.tgtype::integer, t.tgenabled::text, p.proname
  FROM pg_catalog.pg_trigger AS t
  JOIN pg_catalog.pg_class AS c ON c.oid = t.tgrelid
  JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
  JOIN pg_catalog.pg_proc AS p ON p.oid = t.tgfoid
 WHERE n.nspname = 'pigsty_cmdb'
   AND p.pronamespace = n.oid
   AND NOT t.tgisinternal
 ORDER BY c.relname, t.tgname`)
	if err != nil {
		return schemaInvalid("trigger catalog")
	}
	defer rows.Close()
	type trigger struct {
		table, name string
		kind        int
		enabled     string
		function    string
	}
	var got []trigger
	for rows.Next() {
		var item trigger
		if err := rows.Scan(&item.table, &item.name, &item.kind, &item.enabled, &item.function); err != nil {
			return schemaInvalid("trigger catalog")
		}
		got = append(got, item)
	}
	expected := []trigger{
		{table: "schema_migration", name: "schema_migration_append_only", kind: 27, enabled: "O", function: "reject_append_only"},
		{table: "snapshot", name: "snapshot_append_only", kind: 27, enabled: "O", function: "reject_append_only"},
	}
	if rows.Err() != nil || !slices.Equal(got, expected) {
		return schemaInvalid("trigger shape")
	}
	return nil
}

func validateSchemaPrivileges(ctx context.Context, tx pgx.Tx) error {
	var publicGrants, wrongOwners int
	err := tx.QueryRow(ctx, `
WITH namespace AS (
    SELECT oid, nspowner, nspacl
      FROM pg_catalog.pg_namespace
     WHERE nspname = 'pigsty_cmdb'
), grants AS (
    SELECT x.grantee
      FROM namespace AS n,
           LATERAL pg_catalog.aclexplode(COALESCE(n.nspacl, pg_catalog.acldefault('n', n.nspowner))) AS x
    UNION ALL
    SELECT x.grantee
      FROM pg_catalog.pg_class AS c
      JOIN namespace AS n ON n.oid = c.relnamespace,
           LATERAL pg_catalog.aclexplode(COALESCE(c.relacl, pg_catalog.acldefault('r', c.relowner))) AS x
     WHERE c.relkind = 'r'
    UNION ALL
    SELECT x.grantee
      FROM pg_catalog.pg_proc AS p
      JOIN namespace AS n ON n.oid = p.pronamespace,
           LATERAL pg_catalog.aclexplode(COALESCE(p.proacl, pg_catalog.acldefault('f', p.proowner))) AS x
    UNION ALL
    SELECT x.grantee
      FROM pg_catalog.pg_attribute AS a
      JOIN pg_catalog.pg_class AS c ON c.oid = a.attrelid
      JOIN namespace AS n ON n.oid = c.relnamespace,
           LATERAL pg_catalog.aclexplode(a.attacl) AS x
     WHERE a.attnum > 0 AND NOT a.attisdropped AND a.attacl IS NOT NULL
)
SELECT count(*) FILTER (WHERE grantee = 0),
       (SELECT count(*) FROM pg_catalog.pg_class AS c JOIN namespace AS n ON n.oid = c.relnamespace WHERE c.relowner <> n.nspowner)
  FROM grants`).Scan(&publicGrants, &wrongOwners)
	if err != nil || publicGrants != 0 || wrongOwners != 0 {
		return schemaInvalid("privilege boundary")
	}
	return nil
}

func schemaInvalid(part string) error {
	return failure("CMDB_SCHEMA_INVALID", fmt.Sprintf("native schema %s is invalid", part))
}
