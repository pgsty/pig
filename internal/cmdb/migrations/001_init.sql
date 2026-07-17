CREATE SCHEMA pigsty_cmdb;

REVOKE ALL ON SCHEMA pigsty_cmdb FROM PUBLIC;

CREATE TABLE pigsty_cmdb.schema_migration (
    version bigint CONSTRAINT schema_migration_version_positive CHECK (version > 0),
    name text NOT NULL,
    checksum bytea NOT NULL CONSTRAINT schema_migration_checksum_length CHECK (octet_length(checksum) = 32),
    tool_version text NOT NULL,
    applied_at timestamp with time zone NOT NULL,
    CONSTRAINT schema_migration_pkey PRIMARY KEY (version)
);

CREATE TABLE pigsty_cmdb.snapshot (
    revision bigint CONSTRAINT snapshot_revision_positive CHECK (revision > 0),
    parent_revision bigint,
    merge_base_revision bigint NOT NULL,
    semantic_sha256 bytea NOT NULL CONSTRAINT snapshot_semantic_sha256_length CHECK (octet_length(semantic_sha256) = 32),
    payload jsonb NOT NULL,
    origin text NOT NULL CONSTRAINT snapshot_origin_check CHECK (origin IN ('static_load', 'rollback')),
    source_kind text NOT NULL CONSTRAINT snapshot_source_kind_check CHECK (source_kind IN ('static', 'revision')),
    source_locator text,
    source_bytes_sha256 bytea,
    tool_version text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT snapshot_pkey PRIMARY KEY (revision),
    CONSTRAINT snapshot_parent_revision_fkey FOREIGN KEY (parent_revision)
        REFERENCES pigsty_cmdb.snapshot (revision),
    CONSTRAINT snapshot_merge_base_revision_fkey FOREIGN KEY (merge_base_revision)
        REFERENCES pigsty_cmdb.snapshot (revision) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT snapshot_source_consistency CHECK (
        (
            origin = 'static_load'
            AND source_kind = 'static'
            AND source_locator IS NOT NULL
            AND source_bytes_sha256 IS NOT NULL
            AND octet_length(source_bytes_sha256) = 32
            AND merge_base_revision = revision
        )
        OR
        (
            origin = 'rollback'
            AND source_kind = 'revision'
            AND source_locator IS NOT NULL
            AND source_locator ~ '^cmdb-v1:[1-9][0-9]*:sha256:[0-9a-f]{64}$'
            AND source_bytes_sha256 IS NULL
        )
    )
);

CREATE TABLE pigsty_cmdb.state (
    singleton boolean,
    head_revision bigint,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT state_pkey PRIMARY KEY (singleton),
    CONSTRAINT state_singleton_true CHECK (singleton),
    CONSTRAINT state_head_revision_fkey FOREIGN KEY (head_revision)
        REFERENCES pigsty_cmdb.snapshot (revision)
);

INSERT INTO pigsty_cmdb.state (singleton, head_revision, updated_at)
VALUES (true, NULL, clock_timestamp());

CREATE FUNCTION pigsty_cmdb.reject_append_only()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
AS $function$
BEGIN
    RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'pigsty_cmdb append-only relation';
END;
$function$;

CREATE TRIGGER schema_migration_append_only
BEFORE UPDATE OR DELETE ON pigsty_cmdb.schema_migration
FOR EACH ROW EXECUTE FUNCTION pigsty_cmdb.reject_append_only();

CREATE TRIGGER snapshot_append_only
BEFORE UPDATE OR DELETE ON pigsty_cmdb.snapshot
FOR EACH ROW EXECUTE FUNCTION pigsty_cmdb.reject_append_only();

REVOKE ALL ON ALL TABLES IN SCHEMA pigsty_cmdb FROM PUBLIC;
REVOKE ALL ON FUNCTION pigsty_cmdb.reject_append_only() FROM PUBLIC;
