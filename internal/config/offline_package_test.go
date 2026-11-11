package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveOfflinePackagePathSelection(t *testing.T) {
	const legacy = "/tmp/pkg.tgz"
	for _, tc := range []struct {
		name, input, want string
		primary, fallback error
		calls             []string
		wantErr           error
	}{
		{name: "primary", want: DefaultOfflinePackagePath, calls: []string{DefaultOfflinePackagePath}},
		{name: "legacy", primary: os.ErrNotExist, want: legacy, calls: []string{DefaultOfflinePackagePath, legacy}},
		{name: "missing", primary: os.ErrNotExist, fallback: os.ErrNotExist, want: DefaultOfflinePackagePath, wantErr: os.ErrNotExist, calls: []string{DefaultOfflinePackagePath, legacy}},
		{name: "primary inaccessible", primary: os.ErrPermission, want: DefaultOfflinePackagePath, wantErr: os.ErrPermission, calls: []string{DefaultOfflinePackagePath}},
		{name: "legacy inaccessible", primary: os.ErrNotExist, fallback: os.ErrPermission, want: legacy, wantErr: os.ErrPermission, calls: []string{DefaultOfflinePackagePath, legacy}},
		{name: "explicit default", input: DefaultOfflinePackagePath, primary: os.ErrNotExist, want: DefaultOfflinePackagePath, wantErr: os.ErrNotExist, calls: []string{DefaultOfflinePackagePath}},
		{name: "explicit custom", input: "/custom/pkg.tgz", primary: os.ErrNotExist, want: "/custom/pkg.tgz", wantErr: os.ErrNotExist, calls: []string{"/custom/pkg.tgz"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			path, err := resolveOfflinePackagePath(tc.input, func(path string) (os.FileInfo, error) {
				calls = append(calls, path)
				if path == legacy {
					return nil, tc.fallback
				}
				return nil, tc.primary
			})
			if path != tc.want || !errors.Is(err, tc.wantErr) || !reflect.DeepEqual(calls, tc.calls) {
				t.Fatalf("got %q, %v, calls=%v; want %q, %v, calls=%v", path, err, calls, tc.want, tc.wantErr, tc.calls)
			}
		})
	}
}

func TestResolveOfflinePackagePathPreservesUnsafeCandidate(t *testing.T) {
	for _, kind := range []string{"directory", "dangling symlink", "unreadable file"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pkg.tgz")
			var err error
			switch kind {
			case "directory":
				err = os.Mkdir(path, 0o700)
			case "dangling symlink":
				err = os.Symlink(path+".missing", path)
			case "unreadable file":
				err = os.WriteFile(path, nil, 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := ResolveOfflinePackagePath(path)
			if got != path || err != nil {
				t.Fatalf("got %q, %v", got, err)
			}
			got, err = resolveOfflinePackagePath("", func(candidate string) (os.FileInfo, error) {
				if candidate != DefaultOfflinePackagePath {
					t.Fatal("unsafe primary triggered fallback")
				}
				return os.Lstat(path)
			})
			if got != DefaultOfflinePackagePath || err != nil {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}
