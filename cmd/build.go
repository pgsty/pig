package cmd

import (
	"fmt"
	"pig/cli/build"
	"pig/cli/ext"
	"pig/cli/repo"
	"pig/internal/config"
	"pig/internal/output"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	buildPgrxVer    string
	buildPgrxPg     string
	buildPgrxBeta   bool
	buildDepPg      string
	buildPkgPg      string
	buildPkgNoDebug bool
	buildPkgSymbol  bool // deprecated compatibility flag; debug packages are now the default
	buildAllPg      string
	buildAllNoDebug bool
	buildAllSymbol  bool // deprecated compatibility flag; debug packages are now the default
	buildGetForce   bool
	buildSpecForce  bool
	buildRustYes    bool
	buildRustMirror bool
	buildSpecMirror bool
	buildGetMirror  bool
	buildPkgMirror  bool
	buildRepoBeta   bool
	buildRepoMirror bool
	buildToolBeta   bool
)

// buildCmd represents the top-level `build` command
var buildCmd = &cobra.Command{
	Use:         "build",
	Short:       "Build Postgres Extension",
	Annotations: ancsAnn("pig build", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Aliases:     []string{"b"},
	GroupID:     "pgext",
	Example: `pig build - Build Postgres Extension

Environment Setup:
  pig build spec                   # init build spec and directory (~ext)
  pig build repo                   # init build repo (=repo set -ru)
  pig build repo --beta            # init build repo with PostgreSQL beta repo
  pig build tool  [mini|full|...]  # init build toolset
  pig build rust  [-y] [-m]        # install Rust toolchain
  pig build pgrx  [-v <ver>] [-b]  # install & init pgrx (` + build.DefaultPgrxVersion + `)
  pig build proxy                  # install or verify Xray
  pig build proxy client 'vless://...' # configure HTTP/SOCKS on 127.0.0.1:12345
  pig build proxy server --host HOST --target HOST:443 # configure a REALITY server
  pig build proxy id@host:port     # legacy VMess client

Package Building:
  pig build pkg   [ext|pkg...]     # complete pipeline: get + dep + ext
  pig build get   [ext|pkg...]     # download extension source tarball
  pig build dep   [ext|pkg...]     # install extension build dependencies
  pig build ext   [ext|pkg...]     # build extension package

Quick Start:
  pig build spec                   # setup build spec and directory
  pig build pkg citus              # build citus extension

`, PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := commandModulePreRun(cmd, args); err != nil {
			return err
		}
		return ext.ReloadCatalog()
	},
}

// buildRepoCmd represents the `build repo` command
var buildRepoCmd = &cobra.Command{
	Use:         "repo [module...]",
	Short:       "Initialize required repos",
	Annotations: ancsAnn("pig build repo", "action", "volatile", "unsafe", true, "low", "none", "root", 60000),
	Aliases:     []string{"r"},
	Args:        cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := repo.ConfigureRepos(buildRepoModules(args, buildRepoBeta), repo.AddOptions{
			Region: "",
			Remove: true,
			Update: true,
			Mirror: buildRepoMirror,
		})
		return handleAuxResult(result)
	},
}

// buildInitCmd represents the `build init` command
var buildToolCmd = &cobra.Command{
	Use:         "tool [mode]",
	Short:       "Initialize build tools",
	Annotations: ancsAnn("pig build tool", "action", "volatile", "unsafe", true, "low", "none", "root", 60000),
	Aliases:     []string{"t"},
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleBuild, "pig build tool", args, nil, func() error {
			var mode string
			if len(args) > 0 {
				mode = args[0]
			}
			logrus.Infof("Init pg ext build env in %s mode", mode)
			return build.InstallBuildToolsWithBeta(mode, buildToolBeta)
		})
	},
}

func buildRepoModules(args []string, includeBeta bool) []string {
	if !includeBeta {
		return args
	}
	modules := append([]string(nil), args...)
	if len(modules) == 0 {
		modules = []string{"all"}
	}
	if !moduleArgsContain(modules, "beta") {
		modules = append(modules, "beta")
	}
	return modules
}

func moduleArgsContain(args []string, target string) bool {
	for _, arg := range args {
		for _, part := range strings.Split(arg, ",") {
			if strings.TrimSpace(part) == target {
				return true
			}
		}
	}
	return false
}

// buildProxyCmd represents the `build proxy` command
var buildProxyCmd = &cobra.Command{
	Use:         "proxy [id@remote] [local]",
	Short:       "Initialize build proxy",
	Long:        "Install or verify Xray with no arguments. Use client/server for VLESS/REALITY/Vision setup. The historical id@host:port [local] form retains its VMess/V2Ray behavior and default listener 127.0.0.1:12345.",
	Annotations: ancsAnn("pig build proxy", "action", "volatile", "unsafe", true, "low", "none", "root", 60000),
	Args:        cobra.MaximumNArgs(2),
	Aliases:     []string{"x"},
	RunE: func(cmd *cobra.Command, args []string) error {
		structuredArgs := append([]string(nil), args...)
		if len(structuredArgs) > 0 {
			structuredArgs[0] = "<redacted>"
		}
		return runLegacyStructuredSensitiveWithNextActions(legacyModuleBuild, "pig build proxy", structuredArgs, nil, nil, func() error {
			var remote, local string
			if len(args) == 0 {
				return build.InstallProxy()
			} else {
				if len(args) == 2 {
					remote, local = args[0], args[1]
				} else {
					remote, local = args[0], "127.0.0.1:12345"
				}
			}
			return build.SetupProxy(remote, local)
		})
	},
}

var buildProxyClientCmd = newBuildProxyClientCommand()
var buildProxyServerCmd = newBuildProxyServerCommand()

func newBuildProxyClientCommand() *cobra.Command {
	var opts build.ProxyClientOptions
	var plan bool
	command := &cobra.Command{
		Use:         "client [vless-uri]",
		Short:       "Set up an Xray HTTP/SOCKS client",
		Long:        "Install Xray, configure VLESS/REALITY/Vision, start its service, and verify HTTP and SOCKS HTTPS requests. Connection input is a URI, --from FILE/-, or connection flags. The default listener is 127.0.0.1:12345; reruns preserve an existing listener when --listen is omitted.",
		Annotations: ancsAnn("pig build proxy client", "action", "volatile", "unsafe", true, "medium", "none", proxyClientOSUser(), 120000),
		Args:        cobra.MaximumNArgs(1),
		Example:     "pig build proxy client 'vless://...'\npig build proxy client --from -\npig build proxy client --server proxy.example.com:443 --id UUID --sni www.example.net --public-key KEY --short-id ID --pqv KEY",
	}
	command.RunE = func(cmd *cobra.Command, args []string) error {
		opts.URI = ""
		if len(args) == 1 {
			opts.URI = args[0]
		}
		opts.ListenSet = cmd.Flags().Changed("listen")
		if plan {
			p, err := build.PlanXrayClient(opts, cmd.InOrStdin())
			if err != nil {
				return proxyCommandError("client", err)
			}
			return handlePlanOutput(p)
		}
		if opts.Replace && !opts.Yes {
			return proxyCommandError("client", fmt.Errorf("replacement requires --replace --yes"))
		}
		result, err := build.SetupXrayClient(opts, cmd.InOrStdin())
		if err != nil {
			return proxyCommandError("client", err)
		}
		return handleAuxResult(output.OK("Xray client ready", result))
	}
	f := command.Flags()
	f.StringVar(&opts.From, "from", "", "read one VLESS URI from a file or '-' for stdin")
	f.StringVar(&opts.Server, "server", "", "public server endpoint (host:port)")
	f.StringVar(&opts.ID, "id", "", "client UUID (credential)")
	f.StringVar(&opts.SNI, "sni", "", "REALITY server name")
	f.StringVar(&opts.PublicKey, "public-key", "", "REALITY client authentication key")
	f.StringVar(&opts.ShortID, "short-id", "", "REALITY short ID")
	f.StringVar(&opts.PQV, "pqv", "", "ML-DSA-65 verification key")
	f.StringVar(&opts.Listen, "listen", build.DefaultProxyListen, "local listener; preserve existing listener if omitted")
	f.BoolVar(&plan, "plan", false, "preview without installing, writing, or starting services")
	f.BoolVar(&opts.Replace, "replace", false, "replace a different or unsupported client configuration")
	f.BoolVarP(&opts.Yes, "yes", "y", false, "confirm client configuration replacement")
	return command
}

func newBuildProxyServerCommand() *cobra.Command {
	var opts build.ProxyServerOptions
	var plan bool
	command := &cobra.Command{
		Use:         "server",
		Short:       "Set up or export an Xray REALITY server",
		Long:        "Set up a Linux VLESS/REALITY/Vision server. Repeated setup preserves existing credentials and clients. Direct listeners default to 0.0.0.0:443; --proxy-protocol defaults to 127.0.0.1:9443 behind an existing trusted frontend. --export-only reads existing configuration without changing the deployment.",
		Annotations: ancsAnn("pig build proxy server", "action", "volatile", "unsafe", true, "medium", "none", "root", 120000),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("server does not accept positional arguments")
			}
			return nil
		},
		Example: "pig build proxy server --host proxy.example.com --target www.example.net:443 --export -\npig build proxy server --host proxy.example.com --target www.example.net:443 --proxy-protocol --export -\npig build proxy server --host proxy.example.com --export-only --export -",
	}
	command.RunE = func(cmd *cobra.Command, args []string) error {
		opts.ListenSet, opts.ProxyProtocolSet = cmd.Flags().Changed("listen"), cmd.Flags().Changed("proxy-protocol")
		if opts.Export != "" && config.IsStructuredOutput() {
			return proxyCommandError("server", fmt.Errorf("credential export requires text output; omit --output json/yaml"))
		}
		if plan {
			p, err := build.PlanXrayServer(opts)
			if err != nil {
				return proxyCommandError("server", err)
			}
			return handlePlanOutput(p)
		}
		result, err := build.SetupXrayServer(opts, cmd.OutOrStdout())
		if err != nil {
			return proxyCommandError("server", err)
		}
		// Explicit export must leave stdout as exactly one URI, with diagnostics on stderr.
		if opts.Export != "" {
			return nil
		}
		return handleAuxResult(output.OK("Xray server ready locally; public ingress not yet verified", result))
	}
	f := command.Flags()
	f.StringVar(&opts.Host, "host", "", "advertised public server domain or IP")
	f.IntVar(&opts.Port, "port", 443, "advertised public server port")
	f.StringVar(&opts.Target, "target", "", "REALITY camouflage TLS endpoint (host:port)")
	f.StringVar(&opts.SNI, "sni", "", "allowed server name; first setup defaults to target hostname")
	f.StringVar(&opts.Listen, "listen", "", "bind endpoint; preserve existing or default to direct/public port or backend 127.0.0.1:9443")
	f.BoolVar(&opts.ProxyProtocol, "proxy-protocol", false, "receive PROXY protocol on loopback behind an existing trusted frontend")
	f.StringVar(&opts.Export, "export", "", "explicitly export connection URI to a 0600 file or '-' for stdout")
	f.BoolVar(&opts.ExportOnly, "export-only", false, "read existing server and export its connection without changing configuration or service")
	f.BoolVar(&plan, "plan", false, "preview without installing, generating credentials, writing, or restarting")
	return command
}

func proxyClientOSUser() string {
	if runtime.GOOS == "darwin" {
		return "current"
	}
	return "root"
}

func proxyCommandError(role string, err error) error {
	return handleAuxResult(output.Fail(output.GenericOpFailed(legacyModuleBuild), "Xray "+role+" setup failed").WithDetail(err.Error()))
}

// buildRustCmd represents the `build rust` command
var buildRustCmd = &cobra.Command{
	Use:         "rust",
	Short:       "Install Rust toolchain",
	Annotations: ancsAnn("pig build rust", "action", "volatile", "unsafe", true, "low", "none", "root", 60000),
	Aliases:     []string{"rs"},
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleBuild, "pig build rust", args, map[string]interface{}{
			"yes":    buildRustYes,
			"mirror": buildRustMirror,
		}, func() error {
			return build.SetupRust(buildRustYes, buildRustMirror)
		})
	},
}

// buildPgrxCmd represents the `build pgrx` command
var buildPgrxCmd = &cobra.Command{
	Use:         "pgrx",
	Short:       "Install and initialize pgrx (requires Rust)",
	Annotations: ancsAnn("pig build pgrx", "action", "volatile", "unsafe", true, "low", "none", "root", 60000),
	Aliases:     []string{"rx"},
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleBuild, "pig build pgrx", args, map[string]interface{}{
			"pgrx": buildPgrxVer,
			"pg":   buildPgrxPg,
			"beta": buildPgrxBeta,
		}, func() error {
			return build.SetupPgrx(buildPgrxVer, buildPgrxPg, buildPgrxBeta)
		})
	},
}

// buildSpecCmd represents the `build spec` command
var buildSpecCmd = &cobra.Command{
	Use:         "spec",
	Short:       "Initialize building spec repo",
	Annotations: ancsAnn("pig build spec", "action", "volatile", "safe", true, "low", "none", "root", 60000),
	Long:        "Download and sync build spec repository via tarball and incremental rsync",
	Aliases:     []string{"s"},
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleBuild, "pig build spec", args, map[string]interface{}{
			"force":  buildSpecForce,
			"mirror": buildSpecMirror,
		}, func() error {
			return build.SpecDirSetup(buildSpecForce, buildSpecMirror)
		})
	},
}

// buildGetCmd represents the `build get` command
var buildGetCmd = &cobra.Command{
	Use:         "get <pkg...>",
	Short:       "Download source code tarball",
	Annotations: ancsAnn("pig build get", "action", "volatile", "safe", true, "low", "none", "root", 300000),
	Aliases:     []string{"g"},
	Args:        cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleBuild, "pig build get", args, map[string]interface{}{
			"force":  buildGetForce,
			"mirror": buildGetMirror,
		}, func() error {
			return build.DownloadSources(args, buildGetForce, buildGetMirror)
		})
	},
}

// buildDepCmd represents the `build dep` command
var buildDepCmd = &cobra.Command{
	Use:         "dep <pkg...>",
	Short:       "Install extension build dependencies",
	Annotations: ancsAnn("pig build dep", "action", "volatile", "unsafe", true, "low", "none", "root", 300000),
	Aliases:     []string{"d"},
	Args:        cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleBuild, "pig build dep", args, map[string]interface{}{
			"pg": buildDepPg,
		}, func() error {
			return build.InstallDepsList(args, buildDepPg)
		})
	},
}

// buildExtCmd represents the `build ext` command
var buildExtCmd = &cobra.Command{
	Use:         "ext <pkg...>",
	Short:       "Build extension package",
	Annotations: ancsAnn("pig build ext", "action", "volatile", "unsafe", true, "low", "none", "root", 300000),
	Aliases:     []string{"e"},
	Args:        cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleBuild, "pig build ext", args, map[string]interface{}{
			"pg":     buildPkgPg,
			"nodbg":  buildPkgNoDebug,
			"symbol": !buildPkgNoDebug,
		}, func() error {
			return build.BuildExtensions(args, buildPkgPg, buildPkgNoDebug)
		})
	},
}

// buildPkgCmd represents the `build pkg` command
var buildPkgCmd = &cobra.Command{
	Use:         "pkg <pkg...>",
	Short:       "Complete build pipeline: get, dep, ext",
	Annotations: ancsAnn("pig build pkg", "action", "volatile", "unsafe", true, "low", "none", "root", 300000),
	Long:        "Execute complete build pipeline for extensions: download source, install dependencies, and build package",
	Aliases:     []string{"p"},
	Args:        cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleBuild, "pig build pkg", args, map[string]interface{}{
			"pg":     buildAllPg,
			"nodbg":  buildAllNoDebug,
			"symbol": !buildAllNoDebug,
			"mirror": buildPkgMirror,
		}, func() error {
			return build.BuildPackages(args, buildAllPg, buildAllNoDebug, buildPkgMirror)
		})
	},
}

func init() {
	buildProxyCmd.AddCommand(buildProxyClientCmd, buildProxyServerCmd)
	// Parse build flags
	buildRepoCmd.Flags().BoolVarP(&buildRepoMirror, "mirror", "m", false, "prefer China mirrors")
	buildRepoCmd.Flags().BoolVarP(&buildRepoBeta, "beta", "b", false, "include PostgreSQL beta repository")
	buildToolCmd.Flags().BoolVarP(&buildToolBeta, "beta", "b", false, "include PostgreSQL beta build packages")
	buildPgrxCmd.PersistentFlags().StringVarP(&buildPgrxVer, "pgrx", "v", build.DefaultPgrxVersion, "pgrx version to install")
	buildPgrxCmd.Flags().BoolVarP(&buildPgrxBeta, "beta", "b", false, "include PostgreSQL beta pg_config link")
	buildRustCmd.PersistentFlags().BoolVarP(&buildRustYes, "yes", "y", false, "enforce rust re-installation")
	buildRustCmd.Flags().BoolVarP(&buildRustMirror, "mirror", "m", false, "use China mirrors for rustup and Cargo")

	// Add pgrx specific flags
	buildPgrxCmd.Flags().StringVar(&buildPgrxPg, "pg", "", "comma-separated PG versions to init (e.g. '18,17,16'), 'init' for no args, or auto-detect if empty")

	// Add spec specific flags
	buildSpecCmd.Flags().BoolVarP(&buildSpecForce, "force", "f", false, "force re-download tarball even if exists")
	buildSpecCmd.Flags().BoolVarP(&buildSpecMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")

	// Add get specific flags
	buildGetCmd.Flags().BoolVarP(&buildGetForce, "force", "f", false, "force download even if file exists")
	buildGetCmd.Flags().BoolVarP(&buildGetMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")

	// Add dep specific flags
	buildDepCmd.Flags().StringVar(&buildDepPg, "pg", "", "comma-separated PG versions (e.g. '17,16'), auto-detect from extension if empty")

	// Add ext specific flags (formerly pkg)
	buildExtCmd.Flags().StringVar(&buildPkgPg, "pg", "", "comma-separated PG versions (e.g. '17,16'), auto-detect from extension if empty")
	buildExtCmd.Flags().BoolVar(&buildPkgNoDebug, "nodbg", false, "disable automatic debug packages (RPM debuginfo/debugsource; DEB dbgsym)")
	buildExtCmd.Flags().BoolVarP(&buildPkgSymbol, "symbol", "s", false, "build with debug symbols")
	_ = buildExtCmd.Flags().MarkDeprecated("symbol", "debug packages are built by default; omit --symbol or use --nodbg to disable them")

	// Add pkg specific flags (formerly all)
	buildPkgCmd.Flags().StringVar(&buildAllPg, "pg", "", "comma-separated PG versions (e.g. '17,16'), auto-detect from extension if empty")
	buildPkgCmd.Flags().BoolVar(&buildAllNoDebug, "nodbg", false, "disable automatic debug packages (RPM debuginfo/debugsource; DEB dbgsym)")
	buildPkgCmd.Flags().BoolVarP(&buildAllSymbol, "symbol", "s", false, "build with debug symbols")
	_ = buildPkgCmd.Flags().MarkDeprecated("symbol", "debug packages are built by default; omit --symbol or use --nodbg to disable them")
	buildPkgCmd.Flags().BoolVarP(&buildPkgMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")

	// Add subcommands
	buildCmd.AddCommand(
		buildSpecCmd,
		buildRepoCmd,
		buildToolCmd,
		buildRustCmd,
		buildPgrxCmd,
		buildProxyCmd,
		buildGetCmd,
		buildDepCmd,
		buildExtCmd,
		buildPkgCmd,
	)
}
