package inventory

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUserRolesStringAndObjectCompatibility(t *testing.T) {
	for _, tt := range []struct {
		roles string
		valid bool
	}{
		{`[dbrole_readonly, {name: dbrole_admin, admin: true, inherit: false, set: null, state: grant}]`, true},
		{`[{name: dbrole_readonly, state: revoke}, {name: dbrole_old, state: absent}]`, true},
		{`[]`, true}, {`[""]`, false}, {`[42]`, false}, {`[{admin: true}]`, false}, {`[{name: ""}]`, false},
		{`[{name: role, admin: "yes"}]`, false}, {`[{name: role, state: invalid}]`, false}, {`[{name: role, typo: true}]`, false},
	} {
		t.Run(tt.roles, func(t *testing.T) {
			source := fmt.Sprintf("all:\n  vars:\n    admin_ip: 127.0.0.1\n    pg_users:\n      - name: app\n        roles: %s\n", tt.roles)
			doc, err := Parse("roles.yml", []byte(source))
			if err != nil {
				t.Fatal(err)
			}
			diagnostics := doc.Validate(PigstyRules)
			if HasErrors(diagnostics) == tt.valid {
				t.Fatalf("roles valid=%v diagnostics=%+v", tt.valid, diagnostics)
			}
		})
	}
}

func TestAmbiguousStringsSurviveEditAndSerialization(t *testing.T) {
	// 010 is a retained control case, not evidence that it was previously broken.
	for _, value := range []string{"1:30", "010", "yes", "2001-12-15", "2001-12-15T02:59:43.1Z", "2001-1-5 2:59:43 -5", "2001-13-99", "2001-1-5", "1:30.5", "plain"} {
		for _, flow := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/flow=%v", value, flow), func(t *testing.T) {
				source := "all:\n  vars:\n    value: old # preserve\n"
				if flow {
					source = "all: {vars: {value: old}}\n"
				}
				doc, err := Parse("scalars.yml", []byte(source))
				if err != nil {
					t.Fatal(err)
				}
				editor := doc.Edit()
				if err := editor.Set(AllVars("value"), value); err != nil {
					t.Fatal(err)
				}
				committed, _, err := editor.Commit()
				if err != nil {
					t.Fatal(err)
				}
				reparsed, err := Parse("roundtrip.yml", writeDocument(t, committed))
				if err != nil {
					t.Fatal(err)
				}
				got, ok := reparsed.Lookup(AllVars("value"))
				if !ok || got.Scalar == nil || got.Scalar.Kind != StringScalar || got.Scalar.Value != value {
					t.Fatalf("semantic roundtrip changed %q: %+v", value, got)
				}
				var decoded struct {
					All struct {
						Vars map[string]any
					}
				}
				if err := yaml.Unmarshal(committed.Bytes(), &decoded); err != nil {
					t.Fatal(err)
				}
				if got := decoded.All.Vars["value"]; got != value {
					t.Fatalf("independent YAML decoding changed %q to %T: %v", value, got, got)
				}
				if got.Scalar.Tag != "!!str" {
					t.Fatalf("string was written with tag %s", got.Scalar.Tag)
				}
				if python := os.Getenv("PIG_TEST_PYYAML"); python != "" {
					// CI uses pinned PyYAML as the independent downstream consumer.
					check := exec.Command(python, "-c", "import sys,yaml; v=yaml.safe_load(sys.stdin)['all']['vars']['value']; assert type(v) is str and v == sys.argv[1], type(v).__name__", value)
					check.Stdin = bytes.NewReader(committed.Bytes())
					if out, err := check.CombinedOutput(); err != nil {
						t.Fatalf("PyYAML string semantics: %v: %s", err, out)
					}
				}
			})
		}
	}
}

func TestTimestampScalarsRemainOpaque(t *testing.T) {
	for _, text := range []string{"2001-12-15", "2001-12-15T02:59:43.1Z", "2001-1-5 2:59:43 -5", "!!timestamp 2001-12-15"} {
		source := []byte("all:\n  vars:\n    value: " + text + " # preserved\n")
		doc, err := Parse("timestamp.yml", source)
		if err != nil {
			t.Fatal(err)
		}
		value, _ := doc.Lookup(AllVars("value"))
		if value.Scalar.Kind != UnknownScalar || !bytes.Equal(doc.Bytes(), source) {
			t.Fatalf("timestamp was treated as a string or normalized: %+v", value.Scalar)
		}
	}
}
