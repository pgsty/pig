package cmdb

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	containerTestFlag   = "PIGSTY_CMDB_CONTAINER_TEST"
	containerImageEnv   = "PIGSTY_CMDB_CONTAINER_IMAGE"
	postgres14Image     = "postgres:14.23-alpine@sha256:f1341c01408dc7278e9d365ed4f860cd3f87dd16b4464ac326fc0f422083a579"
	postgres18Image     = "postgres:18.4-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15"
	officialPostgresRef = "postgres@"
)

var cmdbContainer *containerDatabase

type containerDatabase struct {
	name             string
	database         string
	dsn              string
	selectedPlatform string
	cleanupOnce      sync.Once
}

func TestMain(m *testing.M) {
	if os.Getenv(containerTestFlag) != "1" {
		os.Exit(m.Run())
	}
	image := os.Getenv(containerImageEnv)
	if image == "" {
		image = postgres18Image
	}
	harness, err := startContainerDatabase(image)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CMDB container setup failed:", err)
		os.Exit(1)
	}
	cmdbContainer = harness
	code := m.Run()
	harness.cleanup()
	os.Exit(code)
}

func startContainerDatabase(image string) (*containerDatabase, error) {
	digest, reviewed := reviewedContainerImage(image)
	if !reviewed {
		return nil, fmt.Errorf("unreviewed PostgreSQL test image")
	}
	suffix, err := randomHex(8)
	if err != nil {
		return nil, err
	}
	database := "pig_inventory_cmdb_test_" + suffix
	name := "pig-inventory-cmdb-test-" + suffix
	password := "test-" + suffix

	inspect := exec.Command("docker", "image", "inspect", image,
		"--format", "{{json .RepoDigests}}|{{.Os}}/{{.Architecture}}|{{.Id}}")
	identity, err := inspect.Output()
	if err != nil {
		return nil, fmt.Errorf("pinned image is unavailable locally")
	}
	parts := strings.Split(strings.TrimSpace(string(identity)), "|")
	var repoDigests []string
	if len(parts) != 3 || json.Unmarshal([]byte(parts[0]), &repoDigests) != nil ||
		!slicesContains(repoDigests, officialPostgresRef+digest) {
		return nil, fmt.Errorf("pinned image identity mismatch")
	}

	run := exec.Command(
		"docker", "run", "--detach", "--rm",
		"--name", name,
		"--publish", "127.0.0.1::5432",
		"--env", "POSTGRES_PASSWORD="+password,
		"--env", "POSTGRES_DB="+database,
		image,
	)
	if output, err := run.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("container start failed: %s", strings.TrimSpace(string(output)))
	}
	platformInspect := exec.Command("docker", "image", "inspect", "--platform", parts[1], image,
		"--format", "{{.Descriptor.digest}}")
	platformDigest, err := platformInspect.Output()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(platformDigest)), "sha256:") {
		return nil, fmt.Errorf("selected platform image identity is unavailable")
	}
	harness := &containerDatabase{
		name:             name,
		database:         database,
		selectedPlatform: parts[1] + "@" + strings.TrimSpace(string(platformDigest)),
	}
	fail := func(cause error) (*containerDatabase, error) {
		harness.cleanup()
		return nil, cause
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		ready := exec.Command("docker", "exec", name, "pg_isready", "-U", "postgres", "-d", database)
		if ready.Run() == nil {
			break
		}
		if time.Now().After(deadline) {
			return fail(fmt.Errorf("PostgreSQL readiness timeout"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	portOutput, err := exec.Command("docker", "port", name, "5432/tcp").Output()
	if err != nil {
		return fail(fmt.Errorf("container port lookup failed"))
	}
	host, portText, err := net.SplitHostPort(strings.TrimSpace(string(portOutput)))
	if err != nil || !isLoopbackHost(host) {
		return fail(fmt.Errorf("container is not bound to loopback"))
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fail(fmt.Errorf("container port is invalid"))
	}
	harness.dsn = (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("postgres", password),
		Host:     net.JoinHostPort(host, portText),
		Path:     "/" + database,
		RawQuery: "sslmode=disable",
	}).String()
	if err := harness.guard(); err != nil {
		return fail(err)
	}
	for {
		connection, dialErr := net.DialTimeout("tcp", net.JoinHostPort(host, portText), time.Second)
		if dialErr == nil {
			_ = connection.Close()
			break
		}
		if time.Now().After(deadline) {
			return fail(fmt.Errorf("published PostgreSQL port readiness timeout"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	for {
		connectCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		connection, connectErr := pgx.Connect(connectCtx, harness.dsn)
		cancel()
		if connectErr == nil {
			_ = connection.Close(context.Background())
			break
		}
		if time.Now().After(deadline) {
			return fail(fmt.Errorf("published PostgreSQL connection readiness timeout"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	return harness, nil
}

func reviewedContainerImage(image string) (string, bool) {
	for _, reviewed := range []string{postgres14Image, postgres18Image} {
		if image == reviewed {
			_, digest, ok := strings.Cut(reviewed, "@")
			return digest, ok && strings.HasPrefix(digest, "sha256:")
		}
	}
	return "", false
}

func slicesContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestReviewedContainerImagesAreDigestPinned(t *testing.T) {
	for _, image := range []string{postgres14Image, postgres18Image} {
		digest, ok := reviewedContainerImage(image)
		if !ok || len(digest) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(digest, "sha256:") {
			t.Fatalf("reviewed image is not exactly digest pinned: %q", image)
		}
	}
	for _, image := range []string{"postgres:14.23-alpine", "postgres:18.4-alpine", "postgres:latest"} {
		if _, ok := reviewedContainerImage(image); ok {
			t.Fatalf("unpinned image was reviewed: %q", image)
		}
	}
}

func (c *containerDatabase) guard() error {
	parsed, err := url.Parse(c.dsn)
	if err != nil {
		return fmt.Errorf("test DSN is invalid")
	}
	host := parsed.Hostname()
	if !isLoopbackHost(host) || !strings.HasPrefix(strings.TrimPrefix(parsed.Path, "/"), "pig_inventory_cmdb_test_") ||
		strings.TrimPrefix(parsed.Path, "/") != c.database {
		return fmt.Errorf("destructive test guard rejected database target")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func (c *containerDatabase) cleanup() {
	if c == nil || c.name == "" {
		return
	}
	c.cleanupOnce.Do(func() {
		_ = exec.Command("docker", "rm", "--force", c.name).Run()
	})
}

func openContainerStore(t *testing.T) *Store {
	t.Helper()
	if cmdbContainer == nil {
		t.Skip("set " + containerTestFlag + "=1 to run pinned PostgreSQL tests")
	}
	if err := cmdbContainer.guard(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), ResolvedConnection{
		source: ConnectionURLSource,
		value:  cmdbContainer.dsn,
	}, OpenOptions{Timeouts: Timeouts{Transaction: 15 * time.Second}})
	if err != nil {
		config, parseErr := pgx.ParseConfig(cmdbContainer.dsn)
		if parseErr != nil {
			t.Fatalf("%v; direct parse stage failed (%T)", err, parseErr)
		}
		connection, connectErr := pgx.ConnectConfig(t.Context(), config)
		if connectErr != nil {
			t.Fatalf("%v; direct connect stage failed (%T)", err, connectErr)
		}
		defer connection.Close(t.Context())
		var database, address string
		var oid uint32
		var port int32
		var version int
		var boot time.Time
		scanErr := connection.QueryRow(t.Context(), `
SELECT current_database(), d.oid, COALESCE(inet_server_addr()::text, 'local'),
	   COALESCE(inet_server_port(), 0), current_setting('server_version_num')::integer,
       pg_postmaster_start_time()
  FROM pg_database AS d
 WHERE d.datname = current_database()`).Scan(&database, &oid, &address, &port, &version, &boot)
		t.Fatalf("%v; direct identity stage failed (%T)", err, scanErr)
	}
	t.Cleanup(store.Close)
	return store
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
