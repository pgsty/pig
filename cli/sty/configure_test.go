package sty

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/output"
	"regexp"
	"strings"
	"testing"
)

func TestNormalizeConfigureMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", input: "", want: "meta"},
		{name: "normal", input: "ha/full", want: "ha/full"},
		{name: "backslash", input: `ha\full`, want: "ha/full"},
		{name: "traversal", input: "../meta", wantErr: true},
		{name: "absolute", input: "/tmp/meta", wantErr: true},
		{name: "invalid char", input: "meta$1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeConfigureMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got mode=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("mode=%q, want=%q", got, tt.want)
			}
		})
	}
}

func TestResolveOutputPath(t *testing.T) {
	home := "/tmp/pigsty"
	rel := resolveOutputPath(home, "foo.yml")
	if rel != filepath.Join(home, "foo.yml") {
		t.Fatalf("relative output path mismatch: %q", rel)
	}
	abs := resolveOutputPath(home, "/tmp/bar.yml")
	if abs != "/tmp/bar.yml" {
		t.Fatalf("absolute output path mismatch: %q", abs)
	}
}

func TestMutateTemplateBasic(t *testing.T) {
	content := `all:
  children:
    app:
      vars:
        #docker_registry_mirrors: ["https://example"]
        #PIP_MIRROR_URL: https://example
  vars:
    admin_ip: 10.10.10.10
    region: default
    node_tune: oltp
    node_repo_modules: 'node,infra,pgsql'
    pg_version: 18
    pg_conf: oltp.yml
    pg_packages: [ pg18-main ]
`
	got, _, _, err := mutateTemplate(content, mutationOptions{
		Mode:            "meta",
		PrimaryIP:       "192.168.10.10",
		Region:          "china",
		PGVersion:       17,
		CPUCount:        2,
		LocaleAvailable: false,
	})
	if err != nil {
		t.Fatalf("mutateTemplate error: %v", err)
	}
	if strings.Contains(got, "10.10.10.10") {
		t.Fatalf("expected placeholder IP replaced, got:\n%s", got)
	}
	if !strings.Contains(got, "192.168.10.10") {
		t.Fatalf("expected primary ip in output, got:\n%s", got)
	}
	if !strings.Contains(got, "region: china") {
		t.Fatalf("expected region replacement, got:\n%s", got)
	}
	if !strings.Contains(got, "node_tune: tiny") || !strings.Contains(got, "pg_conf: tiny.yml") {
		t.Fatalf("expected tiny tuning replacement, got:\n%s", got)
	}
	if !strings.Contains(got, "docker_registry_mirrors") || !strings.Contains(got, "PIP_MIRROR_URL") {
		t.Fatalf("expected china mirror uncomment, got:\n%s", got)
	}
	if !strings.Contains(got, "pg_version: 17") || !strings.Contains(got, "pg17-main") {
		t.Fatalf("expected pg version replacement, got:\n%s", got)
	}
	if !strings.Contains(got, "pg_locale: C.UTF-8") {
		t.Fatalf("expected locale settings inserted, got:\n%s", got)
	}
}

func TestMutateTemplatePG19EnablesBetaRepoModule(t *testing.T) {
	content := `all:
  vars:
    node_repo_modules: 'node,infra,pgsql'
    pg_version: 18
    pg_packages: [ pg18-main ]
`
	got, _, warnings, err := mutateTemplate(content, mutationOptions{
		Mode:      "meta",
		PGVersion: 19,
	})
	if err != nil {
		t.Fatalf("mutateTemplate error: %v", err)
	}
	if !strings.Contains(got, "node_repo_modules: 'node,infra,pgsql,beta'") {
		t.Fatalf("expected beta repo module for PG19, got:\n%s", got)
	}
	if !strings.Contains(got, "pg_version: 19") || !strings.Contains(got, "pg19-main") {
		t.Fatalf("expected pg19 version and package alias replacement, got:\n%s", got)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestMutateTemplatePG19BetaRepoModuleIdempotent(t *testing.T) {
	// node_repo_modules already has beta (like conf/pg19.yml), repo_modules does not:
	// the former must stay single-beta, the latter must gain beta.
	content := `all:
  vars:
    node_repo_modules: 'node,infra,pgsql,beta'
    repo_modules: node,infra,pgsql
    pg_version: 19
    pg_packages: [ pg19-main ]
`
	got, _, warnings, err := mutateTemplate(content, mutationOptions{
		Mode:      "pg19",
		PGVersion: 19,
	})
	if err != nil {
		t.Fatalf("mutateTemplate error: %v", err)
	}
	if strings.Contains(got, "beta,beta") {
		t.Fatalf("beta repo module duplicated, got:\n%s", got)
	}
	if !strings.Contains(got, "node_repo_modules: 'node,infra,pgsql,beta'") {
		t.Fatalf("expected node_repo_modules to keep single beta, got:\n%s", got)
	}
	if !strings.Contains(got, "repo_modules: node,infra,pgsql,beta") {
		t.Fatalf("expected repo_modules to gain beta, got:\n%s", got)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestMutateTemplatePG19BetaRepoModuleOrderVariants(t *testing.T) {
	// Module lists with non-standard order (supabase: node,pgsql,infra and
	// build/oss: infra,node,pgsql) must still gain beta right after pgsql.
	content := `all:
  vars:
    node_repo_modules: node,pgsql,infra   # supabase order
    repo_modules: infra,node,pgsql
    pg_version: 18
    pg_packages: [ pg18-main ]
`
	got, _, warnings, err := mutateTemplate(content, mutationOptions{
		Mode:      "supabase",
		PGVersion: 19,
	})
	if err != nil {
		t.Fatalf("mutateTemplate error: %v", err)
	}
	if !strings.Contains(got, "node_repo_modules: node,pgsql,beta,infra") {
		t.Fatalf("expected beta inserted after pgsql in reordered list, got:\n%s", got)
	}
	if !strings.Contains(got, "repo_modules: infra,node,pgsql,beta") {
		t.Fatalf("expected beta appended after pgsql, got:\n%s", got)
	}
	if !strings.Contains(got, "pg_version: 19") || !strings.Contains(got, "pg19-main") {
		t.Fatalf("expected pg19 version and package alias replacement, got:\n%s", got)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestMutateTemplatePG19BetaRepoModuleMissingWarns(t *testing.T) {
	// rich-style local-only repo modules: version rewrite still applies,
	// but the beta module cannot be enabled and a warning must be raised.
	content := `all:
  vars:
    node_repo_modules: local
    pg_version: 18
    pg_packages: [ pg18-main ]
`
	got, _, warnings, err := mutateTemplate(content, mutationOptions{
		Mode:      "rich",
		PGVersion: 19,
	})
	if err != nil {
		t.Fatalf("mutateTemplate error: %v", err)
	}
	if strings.Contains(got, "beta") {
		t.Fatalf("unexpected beta module injection, got:\n%s", got)
	}
	if !strings.Contains(got, "pg_version: 19") || !strings.Contains(got, "pg19-main") {
		t.Fatalf("expected pg19 version and package alias replacement, got:\n%s", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "beta repo module not enabled") {
		t.Fatalf("expected beta module warning, got: %v", warnings)
	}
}

func TestMutateTemplatePG19BetaRepoModuleIgnoresComments(t *testing.T) {
	// A commented-out module list must neither be rewritten nor suppress
	// the missing-beta warning.
	content := `all:
  vars:
    #node_repo_modules: 'node,infra,pgsql'
    pg_version: 18
`
	got, _, warnings, err := mutateTemplate(content, mutationOptions{
		Mode:      "meta",
		PGVersion: 19,
	})
	if err != nil {
		t.Fatalf("mutateTemplate error: %v", err)
	}
	if !strings.Contains(got, "#node_repo_modules: 'node,infra,pgsql'") {
		t.Fatalf("expected commented module list untouched, got:\n%s", got)
	}
	if strings.Contains(got, "beta") {
		t.Fatalf("unexpected beta injection into comment, got:\n%s", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "beta repo module not enabled") {
		t.Fatalf("expected beta module warning, got: %v", warnings)
	}
}

func TestMutateTemplatePG19SkipsPinnedKernelModes(t *testing.T) {
	content := `all:
  vars:
    node_repo_modules: node,infra,pgsql
    pg_version: 17
`
	for _, mode := range []string{"mssql", "polar"} {
		got, _, warnings, err := mutateTemplate(content, mutationOptions{
			Mode:      mode,
			PGVersion: 19,
		})
		if err != nil {
			t.Fatalf("mutateTemplate error for %s: %v", mode, err)
		}
		if strings.Contains(got, "beta") {
			t.Fatalf("unexpected beta module for pinned-kernel mode %s, got:\n%s", mode, got)
		}
		if !strings.Contains(got, "pg_version: 17") {
			t.Fatalf("expected pinned pg_version kept for %s, got:\n%s", mode, got)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "ignored") {
			t.Fatalf("expected version-ignored warning for %s, got: %v", mode, warnings)
		}
	}
}

func TestMutateTemplatePGMajorTemplateSkipsVersionRewrite(t *testing.T) {
	for _, tt := range []struct {
		mode   string
		locked int
	}{
		{mode: "pg19", locked: 19},
		{mode: "pg20", locked: 20},
		{mode: "pg21", locked: 21},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			content := fmt.Sprintf(`all:
  vars:
    node_repo_modules: node,infra,pgsql,beta
    pg_version: %d
    pg_packages: [ pg%d-main ]
`, tt.locked, tt.locked)
			got, _, warnings, err := mutateTemplate(content, mutationOptions{
				Mode:      tt.mode,
				PGVersion: 18,
			})
			if err != nil {
				t.Fatalf("mutateTemplate error: %v", err)
			}
			if !strings.Contains(got, fmt.Sprintf("pg_version: %d", tt.locked)) {
				t.Fatalf("expected %s to keep pg_version %d, got:\n%s", tt.mode, tt.locked, got)
			}
			if strings.Contains(got, "pg_version: 18") || strings.Contains(got, "pg18-main") {
				t.Fatalf("expected %s to ignore requested PG18, got:\n%s", tt.mode, got)
			}
			if !strings.Contains(got, fmt.Sprintf("pg%d-main", tt.locked)) {
				t.Fatalf("expected %s to keep locked package aliases, got:\n%s", tt.mode, got)
			}
			if len(warnings) != 0 {
				t.Fatalf("unexpected warnings for %s: %v", tt.mode, warnings)
			}
		})
	}
}

func TestMutateTemplateBetaSkippedOnceMajorIsStable(t *testing.T) {
	// GA tripwire: once 19 enters the stable active window, -v 19 must no
	// longer inject the beta repo module.
	oldActive := ext.PostgresActiveMajorVersions
	ext.PostgresActiveMajorVersions = []int{19, 18, 17, 16, 15}
	defer func() { ext.PostgresActiveMajorVersions = oldActive }()

	content := `all:
  vars:
    node_repo_modules: node,infra,pgsql
    pg_version: 18
`
	got, _, warnings, err := mutateTemplate(content, mutationOptions{
		Mode:      "meta",
		PGVersion: 19,
	})
	if err != nil {
		t.Fatalf("mutateTemplate error: %v", err)
	}
	if strings.Contains(got, "beta") {
		t.Fatalf("beta module must not be injected for stable major, got:\n%s", got)
	}
	if !strings.Contains(got, "pg_version: 19") {
		t.Fatalf("expected pg_version updated to 19, got:\n%s", got)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestMutateTemplateProxyInsertion(t *testing.T) {
	content := `all:
  vars:
    region: default
    proxy_env:
      no_proxy: "localhost"
      http_proxy: "http://old-proxy"
`
	got, _, _, err := mutateTemplate(content, mutationOptions{
		Mode:      "meta",
		PrimaryIP: defaultPrimaryIP,
		Region:    "default",
		Proxy: map[string]string{
			"http_proxy":  "http://new-http",
			"https_proxy": "http://new-https",
			"no_proxy":    "localhost,127.0.0.1",
		},
	})
	if err != nil {
		t.Fatalf("mutateTemplate error: %v", err)
	}
	if strings.Contains(got, "old-proxy") {
		t.Fatalf("expected old proxy removed, got:\n%s", got)
	}
	if !strings.Contains(got, `http_proxy: "http://new-http"`) ||
		!strings.Contains(got, `https_proxy: "http://new-https"`) ||
		!strings.Contains(got, `no_proxy: "localhost,127.0.0.1"`) {
		t.Fatalf("expected proxy block inserted, got:\n%s", got)
	}
}

func TestMutateTemplateLocaleNeedsPgVersionLine(t *testing.T) {
	content := `all:
  vars:
    region: default
`
	got, _, _, err := mutateTemplate(content, mutationOptions{
		Mode:            "mssql",
		PrimaryIP:       defaultPrimaryIP,
		Region:          "default",
		LocaleAvailable: true,
	})
	if err != nil {
		t.Fatalf("mutateTemplate error: %v", err)
	}
	if strings.Contains(got, "pg_locale: C.UTF-8") {
		t.Fatalf("unexpected locale injection without pg_version line, got:\n%s", got)
	}
}

func TestMutateTemplatePasswordGeneration(t *testing.T) {
	content := `all:
  vars:
    grafana_admin_password: pigsty
    pg_admin_password: DBUser.DBA
    pg_monitor_password: DBUser.Monitor
    pg_replication_password: DBUser.Replicator
    patroni_password: Patroni.API
    haproxy_admin_password: pigsty
    minio_secret_key: S3User.MinIO
    etcd_root_password: Etcd.Root
    sample_token: DBUser.Meta
`
	got, generated, _, err := mutateTemplate(content, mutationOptions{
		Mode:             "meta",
		PrimaryIP:        defaultPrimaryIP,
		GeneratePassword: true,
	})
	if err != nil {
		t.Fatalf("mutateTemplate error: %v", err)
	}
	if len(generated) == 0 {
		t.Fatal("expected generated secrets metadata")
	}
	for _, token := range []string{"DBUser.Meta", "S3User.Backup", "Vibe.Coding"} {
		if strings.Contains(got, token) {
			t.Fatalf("expected token %q replaced, got:\n%s", token, got)
		}
	}
	re := regexp.MustCompile(`(?m)^\s*grafana_admin_password:\s*([A-Za-z0-9]{24})\s*$`)
	if !re.MatchString(got) {
		t.Fatalf("expected generated password format, got:\n%s", got)
	}
}

func TestConfigureNativeEndToEndAbsoluteOutput(t *testing.T) {
	tmp := t.TempDir()
	confDir := filepath.Join(tmp, "conf")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	template := `all:
  vars:
    admin_ip: 10.10.10.10
    region: default
    node_tune: oltp
    pg_version: 18
    pg_conf: oltp.yml
`
	if err := os.WriteFile(filepath.Join(confDir, "meta.yml"), []byte(template), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	outPath := filepath.Join(tmp, "abs-out.yml")
	locale := false
	result := ConfigureNative(ConfigureOptions{
		PigstyHome:       tmp,
		Mode:             "meta",
		PrimaryIP:        "192.168.0.10",
		Region:           "default",
		OutputFile:       outPath,
		NonInteractive:   true,
		CPUCount:         8,
		LocaleAvailable:  &locale,
		Generate:         false,
		Skip:             false,
		UseProxy:         false,
		DisablePreflight: true,
	})
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got: %+v", result)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	got := string(data)
	if strings.Contains(got, defaultPrimaryIP) {
		t.Fatalf("expected ip replacement in output, got:\n%s", got)
	}
	if !strings.Contains(got, "192.168.0.10") {
		t.Fatalf("expected configured primary ip, got:\n%s", got)
	}
}

func TestConfigureNativePG19SuggestsDedicatedTemplate(t *testing.T) {
	tmp := t.TempDir()
	confDir := filepath.Join(tmp, "conf")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	meta := `all:
  vars:
    region: default
    node_repo_modules: 'node,infra,pgsql'
    pg_version: 18
`
	if err := os.WriteFile(filepath.Join(confDir, "meta.yml"), []byte(meta), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "pg19.yml"), []byte("all: {}\n"), 0644); err != nil {
		t.Fatalf("write pg19 template: %v", err)
	}

	outPath := filepath.Join(tmp, "out.yml")
	locale := false
	result := ConfigureNative(ConfigureOptions{
		PigstyHome:       tmp,
		Mode:             "meta",
		PrimaryIP:        "192.168.0.10",
		PGVersion:        "19",
		OutputFile:       outPath,
		NonInteractive:   true,
		CPUCount:         8,
		LocaleAvailable:  &locale,
		DisablePreflight: true,
	})
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got: %+v", result)
	}
	data, ok := result.Data.(*ConfigureData)
	if !ok {
		t.Fatalf("unexpected result data type: %T", result.Data)
	}
	suggested := false
	for _, w := range data.Warnings {
		if strings.Contains(w, "pig sty conf -c pg19") {
			suggested = true
		}
	}
	if !suggested {
		t.Fatalf("expected pg19 template suggestion warning, got: %v", data.Warnings)
	}

	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(out), "node_repo_modules: 'node,infra,pgsql,beta'") {
		t.Fatalf("expected beta repo module in output, got:\n%s", out)
	}
}

func TestConfigureNativePGMajorTemplateSkipsVersionRewrite(t *testing.T) {
	tmp := t.TempDir()
	confDir := filepath.Join(tmp, "conf")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	template := `all:
  vars:
    region: default
    node_repo_modules: node,infra,pgsql,beta
    pg_version: 19
    pg_packages: [ pg19-main ]
`
	if err := os.WriteFile(filepath.Join(confDir, "pg19.yml"), []byte(template), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	outPath := filepath.Join(tmp, "out.yml")
	locale := false
	result := ConfigureNative(ConfigureOptions{
		PigstyHome:       tmp,
		Mode:             "pg19",
		PrimaryIP:        "192.168.0.10",
		PGVersion:        "16",
		OutputFile:       outPath,
		NonInteractive:   true,
		CPUCount:         8,
		LocaleAvailable:  &locale,
		DisablePreflight: true,
	})
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got: %+v", result)
	}
	data, ok := result.Data.(*ConfigureData)
	if !ok {
		t.Fatalf("unexpected result data type: %T", result.Data)
	}
	if data.PGVersion != "16" {
		t.Fatalf("expected reported pg_version to preserve requested flag, got %q", data.PGVersion)
	}
	if len(data.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", data.Warnings)
	}

	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "pg_version: 19") || strings.Contains(got, "pg_version: 16") {
		t.Fatalf("expected pg19 template to keep pg_version 19, got:\n%s", got)
	}
}

func TestConfigureNativeInvalidVersion(t *testing.T) {
	tmp := t.TempDir()
	confDir := filepath.Join(tmp, "conf")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "meta.yml"), []byte("all:\n  vars:\n    region: default\n"), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	result := ConfigureNative(ConfigureOptions{
		PigstyHome:       tmp,
		PGVersion:        "20",
		DisablePreflight: true,
	})
	if result == nil || result.Success {
		t.Fatalf("expected failure, got: %+v", result)
	}
	if result.Code != output.CodeStyConfigureInvalidArgs {
		t.Fatalf("unexpected code: %d", result.Code)
	}
}

func TestConfigureNativeTemplateNotFound(t *testing.T) {
	tmp := t.TempDir()
	result := ConfigureNative(ConfigureOptions{
		PigstyHome:       tmp,
		Mode:             "meta",
		DisablePreflight: true,
	})
	if result == nil || result.Success {
		t.Fatalf("expected failure, got: %+v", result)
	}
	if result.Code != output.CodeStyConfigureTemplateNotFound {
		t.Fatalf("unexpected code: %d", result.Code)
	}
}

func TestConfigureNativeRejectTraversalMode(t *testing.T) {
	tmp := t.TempDir()
	result := ConfigureNative(ConfigureOptions{
		PigstyHome:       tmp,
		Mode:             "../meta",
		DisablePreflight: true,
	})
	if result == nil || result.Success {
		t.Fatalf("expected failure, got: %+v", result)
	}
	if result.Code != output.CodeStyConfigureInvalidArgs {
		t.Fatalf("unexpected code: %d", result.Code)
	}
}

func TestConfigureDataText(t *testing.T) {
	data := &ConfigureData{
		Mode:         "meta",
		TemplatePath: "/tmp/conf/meta.yml",
		OutputPath:   "/tmp/pigsty.yml",
		Region:       "default",
		PrimaryIP:    "10.10.10.10",
		SSHPort:      "2222",
		PGVersion:    "18",
		Warnings:     []string{"warn-a"},
	}
	text := data.Text()
	for _, expected := range []string{
		"configure pigsty",
		"[ OK ] mode",
		"/tmp/conf/meta.yml",
		"/tmp/pigsty.yml",
		"[ OK ] region",
		"[ OK ] primary_ip",
		"[ OK ] ssh_port",
		"[ OK ] pg_version",
		"[WARN] warn-a",
		"[WARN] don't forget to check it and change passwords!",
		"proceed with ./deploy.yml",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in text output, got:\n%s", expected, text)
		}
	}
}

func TestValidatePGVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{input: "", want: 0},
		{input: "14", want: 14},
		{input: "18", want: 18},
		{input: "19", want: 19},
		{input: "12", wantErr: true},
		{input: "20", wantErr: true},
		{input: "abc", wantErr: true},
	}
	for _, tt := range tests {
		got, err := validatePGVersion(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("expected error for %q, got version=%d", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("version mismatch for %q: got=%d want=%d", tt.input, got, tt.want)
		}
	}
}

func TestResolveTemplatePathGuardsTraversal(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "conf"), 0755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	path, err := resolveTemplatePath(tmp, "meta")
	if err != nil {
		t.Fatalf("resolve safe path failed: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("conf", "meta.yml")) {
		t.Fatalf("unexpected path: %q", path)
	}
	if _, err := resolveTemplatePath(tmp, "../meta"); err == nil {
		t.Fatal("expected traversal path to fail")
	}
}

func TestBuildProxyEnv(t *testing.T) {
	if proxy := buildProxyEnv(false); proxy != nil {
		t.Fatalf("expected nil proxy when disabled, got: %#v", proxy)
	}

	t.Setenv("HTTP_PROXY", "")
	t.Setenv("http_proxy", "http://lowercase-http")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "socks5://all")
	t.Setenv("NO_PROXY", "")
	proxy := buildProxyEnv(true)
	if proxy["http_proxy"] != "http://lowercase-http" {
		t.Fatalf("expected lowercase http_proxy fallback, got: %#v", proxy)
	}
	if proxy["https_proxy"] != "socks5://all" {
		t.Fatalf("expected https_proxy fallback from all_proxy, got: %#v", proxy)
	}
	if proxy["all_proxy"] != "socks5://all" {
		t.Fatalf("expected all_proxy value, got: %#v", proxy)
	}
	if proxy["no_proxy"] != defaultNoProxy {
		t.Fatalf("expected default no_proxy fallback, got: %#v", proxy)
	}
}

func TestUpsertPGVersionAppendsWhenMissing(t *testing.T) {
	content := "all:\n  vars:\n    region: default\n"
	got := upsertPGVersion(content, 17)
	if !strings.Contains(got, "pg_version: 17") {
		t.Fatalf("expected appended pg_version, got:\n%s", got)
	}
}

func TestShouldSetLocale(t *testing.T) {
	if !shouldSetLocale(0, "meta", false) {
		t.Fatal("expected locale enabled when pg version is not specified for meta")
	}
	if shouldSetLocale(16, "mssql", false) {
		t.Fatal("expected locale disabled for mssql without locale support")
	}
	if !shouldSetLocale(16, "mssql", true) {
		t.Fatal("expected locale enabled when locale support exists")
	}
	if !shouldSetLocale(17, "polar", false) {
		t.Fatal("expected locale enabled for pg>=17")
	}
}

func TestInsertLocaleSettingsIdempotent(t *testing.T) {
	content := "all:\n  vars:\n    pg_version: 18\n"
	withLocale := insertLocaleSettings(content)
	if !strings.Contains(withLocale, "pg_locale: C.UTF-8") {
		t.Fatalf("expected locale block inserted, got:\n%s", withLocale)
	}
	again := insertLocaleSettings(withLocale)
	if strings.Count(again, "pg_locale: C.UTF-8") != 1 {
		t.Fatalf("expected locale block inserted only once, got:\n%s", again)
	}

	noPgVersion := "all:\n  vars:\n    region: default\n"
	if got := insertLocaleSettings(noPgVersion); got != noPgVersion {
		t.Fatalf("expected content unchanged without pg_version, got:\n%s", got)
	}
}

func TestRandomPassword(t *testing.T) {
	got, err := randomPassword(32)
	if err != nil {
		t.Fatalf("randomPassword error: %v", err)
	}
	if len(got) != 32 {
		t.Fatalf("unexpected password length: %d", len(got))
	}
	matched, err := regexp.MatchString(`^[A-Za-z0-9]{32}$`, got)
	if err != nil {
		t.Fatalf("regexp error: %v", err)
	}
	if !matched {
		t.Fatalf("password contains non-alphanumeric characters: %q", got)
	}
}
