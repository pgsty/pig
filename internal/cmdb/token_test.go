package cmdb

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRevisionTokenExactGrammar(t *testing.T) {
	valid := "cmdb-v1:42:sha256:" + strings.Repeat("ab", 32)
	token, err := ParseRevisionToken(valid)
	if err != nil {
		t.Fatal(err)
	}
	if token.String() != valid || token.Revision() != 42 || token.Digest() != strings.Repeat("ab", 32) {
		t.Fatalf("token did not round-trip: %q", token.String())
	}
	encoded, err := json.Marshal(token)
	if err != nil || string(encoded) != `"`+valid+`"` {
		t.Fatalf("token JSON is not the exact safe string: %s err=%v", encoded, err)
	}

	invalid := []string{
		"", "cmdb-v1:0:sha256:" + strings.Repeat("ab", 32),
		"cmdb-v1:01:sha256:" + strings.Repeat("ab", 32),
		"cmdb-v1:+1:sha256:" + strings.Repeat("ab", 32),
		"cmdb-v1:1:sha256:" + strings.Repeat("AB", 32),
		"cmdb-v1:1:sha256:" + strings.Repeat("ab", 31),
		"cmdb-v2:1:sha256:" + strings.Repeat("ab", 32),
	}
	for _, value := range invalid {
		if _, err := ParseRevisionToken(value); !IsCode(err, "CMDB_REVISION_TOKEN_INVALID") {
			t.Fatalf("accepted invalid token %q: %v", value, err)
		}
	}
}

func TestStaticLocatorMustAlreadyBeCanonicalAndAbsolute(t *testing.T) {
	if !validCanonicalStaticLocator("/srv/pigsty/inventory") {
		t.Fatal("canonical absolute locator was rejected")
	}
	for _, value := range []string{"", "inventory", "/srv/pigsty/../inventory", "/srv//inventory", "bad\x00path"} {
		if validCanonicalStaticLocator(value) {
			t.Fatalf("accepted non-canonical locator %q", value)
		}
	}
}
