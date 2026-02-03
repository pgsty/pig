package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"pig/internal/config"

	"gopkg.in/yaml.v3"
)

func samplePlan() *Plan {
	return &Plan{
		Command: "pig pitr -t 2026-01-31 01:00:00 --plan",
		Actions: []Action{
			{Step: 1, Description: "Stop Patroni service"},
			{Step: 2, Description: "Ensure PostgreSQL is stopped"},
		},
		Affects: []Resource{
			{Type: "service", Name: "patroni", Impact: "stop"},
			{Type: "data", Name: "/pg/data", Impact: "restore", Detail: "pgbackrest"},
		},
		Expected: "PostgreSQL restored to target time",
		Risks:    []string{"This will overwrite current data"},
	}
}

func TestPlanText(t *testing.T) {
	plan := samplePlan()
	text := plan.Text()
	if text == "" {
		t.Fatal("Text() should not return empty string")
	}
	checks := []string{
		"Execution Plan",
		"Command:",
		"Actions:",
		"[1] Stop Patroni service",
		"Affects:",
		"Expected:",
		"Risks:",
	}
	for _, c := range checks {
		if !strings.Contains(text, c) {
			t.Errorf("Text() missing %q", c)
		}
	}
}

func TestPlanYAMLJSON(t *testing.T) {
	plan := samplePlan()

	yamlData, err := plan.YAML()
	if err != nil {
		t.Fatalf("YAML() error: %v", err)
	}
	var yamlPlan Plan
	if err := yaml.Unmarshal(yamlData, &yamlPlan); err != nil {
		t.Fatalf("YAML unmarshal error: %v", err)
	}
	if yamlPlan.Command != plan.Command {
		t.Errorf("YAML command = %q, want %q", yamlPlan.Command, plan.Command)
	}

	jsonData, err := plan.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}
	var jsonPlan Plan
	if err := json.Unmarshal(jsonData, &jsonPlan); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}
	if jsonPlan.Command != plan.Command {
		t.Errorf("JSON command = %q, want %q", jsonPlan.Command, plan.Command)
	}
}

func TestPlanRenderUnknownFormat(t *testing.T) {
	plan := samplePlan()
	if _, err := plan.Render("bogus"); err == nil {
		t.Fatal("Render() should return error for unknown format")
	}
}

func TestPlanNilReceiver(t *testing.T) {
	var plan *Plan
	if got := plan.Text(); got != "" {
		t.Errorf("Text() on nil receiver should be empty, got %q", got)
	}
	if _, err := plan.YAML(); err == nil {
		t.Error("YAML() on nil receiver should error")
	}
	if _, err := plan.JSON(); err == nil {
		t.Error("JSON() on nil receiver should error")
	}
	if _, err := plan.JSONPretty(); err == nil {
		t.Error("JSONPretty() on nil receiver should error")
	}
	if _, err := plan.Render("text"); err == nil {
		t.Error("Render() on nil receiver should error")
	}
}

func TestPrintPlanTo(t *testing.T) {
	plan := samplePlan()
	buf := &bytes.Buffer{}

	orig := config.OutputFormat
	config.OutputFormat = config.OUTPUT_TEXT
	defer func() { config.OutputFormat = orig }()

	if err := PrintPlanTo(buf, plan); err != nil {
		t.Fatalf("PrintPlanTo() error: %v", err)
	}
	if !strings.Contains(buf.String(), "Execution Plan") {
		t.Errorf("PrintPlanTo() output missing header: %q", buf.String())
	}
}
