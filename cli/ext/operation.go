package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"time"

	"github.com/sirupsen/logrus"
)

type pkgOp int

const (
	pkgOpInstall pkgOp = iota
	pkgOpRemove
	pkgOpUpdate
)

type preparedPkgOp struct {
	StartTime time.Time

	PgVersion int
	Requested []string
	Packages  []string
	PkgToExt  map[string]string
	Failed    []*FailedExtItem
}

type preparePkgOpOptions struct {
	PgVersion             int
	Requested             []string
	ParseVersionSpec      bool
	MacUnsupportedMessage string
}

func prepareExtensionPkgOp(opts preparePkgOpOptions) (*preparedPkgOp, *output.Result) {
	startTime := time.Now()

	if len(opts.Requested) == 0 {
		return nil, output.Fail(output.CodeExtensionInvalidArgs, "no extensions specified")
	}

	pgVer := opts.PgVersion
	if pgVer == 0 {
		logrus.Debugf("using latest postgres version: %d by default", PostgresLatestMajorVersion)
		pgVer = PostgresLatestMajorVersion
	}

	// Check OS support.
	switch config.OSType {
	case config.DistroEL, config.DistroDEB:
		// supported
	case config.DistroMAC:
		return nil, output.Fail(output.CodeExtensionUnsupportedOS, opts.MacUnsupportedMessage)
	default:
		return nil, output.Fail(output.CodeExtensionUnsupportedOS, fmt.Sprintf("unsupported OS: %s", config.OSType))
	}

	// Check Catalog is initialized.
	if Catalog == nil {
		return nil, output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	resolved := ResolveExtensionPackages(pgVer, opts.Requested, opts.ParseVersionSpec)
	failed := make([]*FailedExtItem, 0, len(resolved.NotFound)+len(resolved.NoPackage))
	for _, name := range resolved.NotFound {
		failed = append(failed, &FailedExtItem{
			Name:  name,
			Error: "extension not found in catalog",
			Code:  output.CodeExtensionNotFound,
		})
	}
	for _, name := range resolved.NoPackage {
		failed = append(failed, &FailedExtItem{
			Name:  name,
			Error: fmt.Sprintf("no package available for extension on PG %d", pgVer),
			Code:  output.CodeExtensionNoPackage,
		})
	}

	return &preparedPkgOp{
		StartTime: startTime,
		PgVersion: pgVer,
		Requested: opts.Requested,
		Packages:  resolved.Packages,
		PkgToExt:  resolved.PackageOwner,
		Failed:    failed,
	}, nil
}

func buildPackageManagerCommand(op pkgOp, yes bool, packages []string) []string {
	pkgMgr := PackageManagerCmd()
	cmd := []string{}

	switch op {
	case pkgOpInstall:
		cmd = append(cmd, pkgMgr, "install")
	case pkgOpRemove:
		cmd = append(cmd, pkgMgr, "remove")
	case pkgOpUpdate:
		switch config.OSType {
		case config.DistroDEB:
			cmd = append(cmd, pkgMgr, "install", "--only-upgrade")
		default:
			cmd = append(cmd, pkgMgr, "update")
		}
	}

	if yes {
		cmd = append(cmd, "-y")
	}
	cmd = append(cmd, packages...)
	return cmd
}

func extNameForPackage(pkg string, pkgToExt map[string]string) string {
	if pkgToExt == nil {
		return pkg
	}
	if extName := pkgToExt[pkg]; extName != "" {
		return extName
	}
	return pkg
}

func appendPackageFailures(failed []*FailedExtItem, packages []string, pkgToExt map[string]string, err error, failCode int) []*FailedExtItem {
	if err == nil {
		return failed
	}
	for _, pkg := range packages {
		failed = append(failed, &FailedExtItem{
			Name:    extNameForPackage(pkg, pkgToExt),
			Package: pkg,
			Error:   err.Error(),
			Code:    failCode,
		})
	}
	return failed
}
