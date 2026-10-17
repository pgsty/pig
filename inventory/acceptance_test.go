package inventory

import (
	"fmt"
	"testing"
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
	for _, value := range []string{"1:30", "010", "yes", "2001-12-15", "1:30.5", "plain"} {
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
			})
		}
	}
}
