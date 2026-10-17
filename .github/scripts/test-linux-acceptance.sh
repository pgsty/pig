#!/usr/bin/env bash
# Run only inside a disposable Linux container, as root, with PostgreSQL 18,
# sudo and procps installed. The argument contains postgres.test and utils.test.
set -euo pipefail

test_bins=$(cd "${1:?test binary directory required}" && pwd)
[[ $(id -u) == 0 && $(uname -s) == Linux ]]
pg_bin=$(pg_config --bindir)
"$pg_bin/postgres" --version | grep -Eq 'PostgreSQL\) 18\.'
work=$(mktemp -d /tmp/pig-linux-acceptance.XXXXXX)
reviewer="pig-review-$$"
sudoers="/etc/sudoers.d/$reviewer"
pg_data="$work/private/source"
created_user=false

cleanup() {
    if [[ -f "$pg_data/postmaster.pid" ]]; then
        sudo -u postgres "$pg_bin/pg_ctl" -D "$pg_data" -m immediate -w stop >/dev/null || true
    fi
    if [[ "$created_user" == true ]]; then
        rm -f "$sudoers"
        userdel "$reviewer" 2>/dev/null || true
    fi
    rm -rf "$work"
}
trap cleanup EXIT

chmod 755 "$work"
useradd --no-create-home --shell /bin/sh "$reviewer"
created_user=true
printf '%s ALL=(postgres) NOPASSWD: ALL\n' "$reviewer" > "$sudoers"
chmod 440 "$sudoers"
visudo -cf "$sudoers"
install -d -m 700 -o postgres -g postgres "$work/private" "$pg_data"
install -d -m 775 -o postgres -g postgres /var/run/postgresql
sudo -u postgres "$pg_bin/initdb" -D "$pg_data" --no-locale --auth-local=trust --auth-host=reject
sudo -u postgres "$pg_bin/pg_ctl" -D "$pg_data" -l "$pg_data/server.log" \
    -o '-p 6543 -k /var/run/postgresql -c listen_addresses=' -w start

PIG_TEST_REAL_DBSU=postgres "$test_bins/utils.test" \
    -test.v -test.run '^TestWriteFileAsDBSULinuxIdentity$' | tee "$work/utils.log"
grep -q '^--- PASS: TestWriteFileAsDBSULinuxIdentity ' "$work/utils.log"

sudo -u "$reviewer" env PIG_TEST_CLONE_PGDATA="$pg_data" PIG_TEST_PG_BIN="$pg_bin" \
    PIG_TEST_POSTGRES_PID="$(head -1 "$pg_data/postmaster.pid")" \
    "$test_bins/postgres.test" -test.v \
    -test.run '^(TestCloneLinuxProtectedPGDATA|TestReviewLinuxPrivateForkAndStaleSQLTarget|TestLinuxLivePostgresPermissionDenied|TestMaintenanceLibpqTargetBoundary)$' \
    | tee "$work/postgres.log"
for test_name in TestCloneLinuxProtectedPGDATA TestReviewLinuxPrivateForkAndStaleSQLTarget TestLinuxLivePostgresPermissionDenied TestMaintenanceLibpqTargetBoundary; do
    grep -q "^--- PASS: $test_name " "$work/postgres.log"
done
