package cmdb

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestMigrationAssetsAndAdvisoryLockAreFrozen(t *testing.T) {
	if got := advisoryKeyFromContract(); got != CMDBAdvisoryKey {
		t.Fatalf("advisory key drifted: got %d want %d", got, CMDBAdvisoryKey)
	}
	if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Name != "init" || migrations[0].MinimumPGMajor != 14 {
		t.Fatalf("unexpected migration manifest: %+v", migrations)
	}
	digest := sha256.Sum256(migrations[0].SQL)
	if digest != migrations[0].Checksum {
		t.Fatal("embedded migration checksum is inconsistent")
	}
	if len(hex.EncodeToString(digest[:])) != 64 {
		t.Fatal("migration digest is not SHA-256")
	}
	if got := hex.EncodeToString(digest[:]); got != "1fb054340dce1b3a719f88345a568d032d2da045f89d787be759a5009052d299" {
		t.Fatalf("reviewed migration bytes drifted: %s", got)
	}
}

func TestConstraintCoverageMatchesCatalogContract(t *testing.T) {
	if len(expectedColumns) != 19 {
		t.Fatalf("unexpected logical column count: %d", len(expectedColumns))
	}
	if len(expectedConstraints) != 14 || len(expectedConstraintDefinitions) != 14 {
		t.Fatalf("constraint contract is incomplete: shape=%d definitions=%d", len(expectedConstraints), len(expectedConstraintDefinitions))
	}
	for _, constraint := range expectedConstraints {
		if _, ok := expectedConstraintDefinitions[constraint.Name]; !ok {
			t.Fatalf("constraint %s has no semantic definition check", constraint.Name)
		}
	}
}
