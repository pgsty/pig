package utils

import (
	"reflect"
	"testing"
)

func TestLocalPackageUpgradeCommand(t *testing.T) {
	for _, tc := range []struct {
		osType string
		want   []string
	}{
		{osType: "rpm", want: []string{"rpm", "-U", "--replacepkgs", "--oldpackage", "/tmp/pig.rpm"}},
		{osType: "deb", want: []string{"apt-get", "install", "-y", "--reinstall", "--allow-downgrades", "/tmp/pig.rpm"}},
	} {
		got, err := LocalPackageUpgradeCommand(tc.osType, "/tmp/pig.rpm")
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("LocalPackageUpgradeCommand(%q) = %#v, %v; want %#v", tc.osType, got, err, tc.want)
		}
	}
	if _, err := LocalPackageUpgradeCommand("brew", "/tmp/pig.pkg"); err == nil {
		t.Fatal("unsupported package type was accepted")
	}
}
