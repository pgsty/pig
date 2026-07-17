package sty

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"pig/internal/grafana"
	"pig/internal/output"
)

const dashboardCompatibilityImage = "grafana/grafana:12.3.0"

func TestDashboardDisposableGrafanaCompatibility(t *testing.T) {
	if os.Getenv("PIG_GRAFANA_CONTAINER") != "1" {
		t.Skip("set PIG_GRAFANA_CONTAINER=1 to run the disposable Grafana compatibility gate")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available")
	}
	image := strings.TrimSpace(os.Getenv("PIG_GRAFANA_IMAGE"))
	if image == "" {
		image = dashboardCompatibilityImage
	}
	name := fmt.Sprintf("pig-sty-grafana-%d-%d", os.Getpid(), time.Now().UnixNano())
	password := "container-api-secret"
	run := exec.Command("docker", "run", "-d", "--rm", "--name", name,
		"-p", "127.0.0.1::3000",
		"-e", "GF_SECURITY_ADMIN_USER=admin",
		"-e", "GF_SECURITY_ADMIN_PASSWORD="+password,
		"-e", "GF_USERS_ALLOW_SIGN_UP=false",
		"-e", "GF_SERVER_ROOT_URL=http://localhost:3000/ui",
		"-e", "GF_SERVER_SERVE_FROM_SUB_PATH=true",
		"-e", "GF_LOG_LEVEL=warn",
		image,
	)
	if raw, err := run.CombinedOutput(); err != nil {
		t.Fatalf("start disposable Grafana %s: %v: %s", image, err, strings.TrimSpace(string(raw)))
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})

	portRaw, err := exec.Command("docker", "port", name, "3000/tcp").CombinedOutput()
	if err != nil {
		t.Fatalf("resolve disposable Grafana port: %v: %s", err, strings.TrimSpace(string(portRaw)))
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(string(portRaw)))
	if err != nil {
		t.Fatalf("parse disposable Grafana port %q: %v", strings.TrimSpace(string(portRaw)), err)
	}
	endpoint := "http://" + net.JoinHostPort(host, port) + "/ui"

	manifest := dashboardTestManifest("compat")
	manifest.Datasources = []DashboardManifestDatasource{{
		UID: "ds-compat", Name: "Compatibility", Type: "prometheus",
		Spec: map[string]interface{}{"access": "proxy", "url": "http://127.0.0.1:8428"},
	}}
	manifest.RemovedDashboards = []DashboardManifestTombstone{{UID: "retired", FolderUID: "legacy"}}
	home := writeDashboardTestRepository(t, manifest)
	t.Setenv("GRAFANA_PASSWORD", password)
	options := DashboardOptions{Home: home, Endpoint: endpoint, Username: "admin"}

	deadline := time.Now().Add(60 * time.Second)
	var check *output.Result
	for time.Now().Before(deadline) {
		check = DashboardCheck(options)
		if check.Success {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if check == nil || !check.Success {
		logs, _ := exec.Command("docker", "logs", "--tail", "100", name).CombinedOutput()
		t.Fatalf("disposable Grafana did not become compatible: %v\n%s", check, strings.TrimSpace(string(logs)))
	}

	first := DashboardInit(options)
	if !first.Success {
		t.Fatalf("real Grafana init failed: %s", first)
	}
	second := DashboardInit(options)
	if !second.Success {
		t.Fatalf("real Grafana idempotent init failed: %s", second)
	}
	for _, result := range second.Data.(*DashboardActionData).Results {
		if result.Status != "unchanged" {
			t.Fatalf("second real Grafana init mutated %s %s: %+v", result.ObjectKind, result.UID, result)
		}
	}
	if diff := DashboardDiff(options); !diff.Success {
		t.Fatalf("real Grafana normalized diff failed: %s", diff)
	}
	list := DashboardList(options)
	if !list.Success || !findDashboardObject(t, list.Data.(*DashboardListData).Objects, "dashboard", "compat").Live {
		t.Fatalf("real Grafana list failed: %s", list)
	}

	client, err := grafana.NewClient(grafana.Config{Endpoint: endpoint, Username: "admin", Password: password})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.CreateFolder(ctx, "legacy", "LEGACY"); err != nil {
		t.Fatalf("seed tombstone folder: %v", err)
	}
	if err := client.UpsertDashboard(ctx, map[string]interface{}{"uid": "retired", "title": "Retired", "panels": []interface{}{}}, "legacy"); err != nil {
		t.Fatalf("seed tombstoned dashboard: %v", err)
	}
	pruneOptions := options
	pruneOptions.Prune = true
	if plan := DashboardInit(pruneOptions); plan.Success || plan.Code != output.CodeStyDashboardConfirmationRequired {
		t.Fatalf("real Grafana prune did not fail closed: %s", plan)
	}
	if _, err := client.GetDashboard(ctx, "retired"); err != nil {
		t.Fatalf("real Grafana prune plan mutated tombstone: %v", err)
	}
	pruneOptions.Yes = true
	if result := DashboardInit(pruneOptions); !result.Success {
		t.Fatalf("real Grafana tombstone prune failed: %s", result)
	}
	if _, err := client.GetDashboard(ctx, "retired"); grafana.StatusCode(err) != 404 {
		t.Fatalf("tombstoned dashboard still live after real prune: %v", err)
	}

	if err := client.UpsertDashboard(ctx, map[string]interface{}{"uid": "compat-copy", "title": "Compatibility copy", "panels": []interface{}{}}, "infra"); err != nil {
		t.Fatalf("seed copied dashboard: %v", err)
	}
	if plan := DashboardClean(options); plan.Success || plan.Code != output.CodeStyDashboardConfirmationRequired {
		t.Fatalf("real Grafana clean did not fail closed: %s", plan)
	}
	cleanOptions := options
	cleanOptions.Yes = true
	if result := DashboardClean(cleanOptions); !result.Success {
		t.Fatalf("real Grafana clean failed: %s", result)
	}
	if _, err := client.GetDashboard(ctx, "compat"); grafana.StatusCode(err) != 404 {
		t.Fatalf("managed dashboard still live after real clean: %v", err)
	}
	if _, err := client.GetDashboard(ctx, "compat-copy"); err != nil {
		t.Fatalf("real clean deleted copied dashboard: %v", err)
	}
	state, err := fetchDashboardLiveState(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := state.Folders["infra"]; !exists {
		t.Fatal("real clean deleted a placement folder containing a copied dashboard")
	}
	if _, exists := state.Datasources["ds-compat"]; exists {
		t.Fatal("managed datasource still live after real clean")
	}
}
