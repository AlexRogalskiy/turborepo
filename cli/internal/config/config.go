package config

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vercel/turborepo/cli/internal/client"
	"github.com/vercel/turborepo/cli/internal/fs"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/kelseyhightower/envconfig"
	"github.com/mattn/go-isatty"
	"github.com/mitchellh/cli"
)

const (
	// EnvLogLevel is the environment log level
	EnvLogLevel = "TURBO_LOG_LEVEL"
)

// IsCI returns true if running in a CI/CD environment
func IsCI() bool {
	return !isatty.IsTerminal(os.Stdout.Fd()) || os.Getenv("CI") != ""
}

// Config is a struct that contains user inputs and our logger
type Config struct {
	Logger hclog.Logger
	// Bearer token
	Token string
	// vercel.com / remote cache team id
	TeamId string
	// vercel.com / remote cache team slug
	TeamSlug string
	// Backend API URL
	ApiUrl string
	// Login URL
	LoginUrl string
	// Backend retryable http client
	ApiClient *client.ApiClient
	// Turborepo CLI Version
	TurboVersion string
	Cache        *CacheConfig
	// turbo.json or legacy turbo config from package.json
	TurboConfigJSON *fs.TurboConfigJSON
	// package.json at the root of the repo
	RootPackageJSON *fs.PackageJSON
	// Current Working Directory
	Cwd string
}

// IsLoggedIn returns true if we have a token and either a team id or team slug
func (c *Config) IsLoggedIn() bool {
	return c.Token != "" && (c.TeamId != "" || c.TeamSlug != "")
}

// CacheConfig
type CacheConfig struct {
	// Number of async workers
	Workers int
	// Cache directory
	Dir string
}

// ParseAndValidate parses the cmd line flags / env vars, and verifies that all required
// flags have been set. Users can pass in flags when calling a subcommand, or set env vars
// with the prefix 'TURBO_'. If both values are set, the env var value will be used.
func ParseAndValidate(args []string, ui cli.Ui, turboVersion string) (c *Config, err error) {

	// Special check for ./turbo invocation without any args
	// Return the help message
	if len(args) == 0 {
		args = append(args, "--help")
	}

	// Pop the subcommand into 'cmd'
	// flags.Parse does not work when the subcommand is included
	cmd, inputFlags := args[0], args[1:]

	// Special check for help commands
	// command is ./turbo --help or --version
	if len(inputFlags) == 0 && (cmd == "help" || cmd == "--help" || cmd == "-help" || cmd == "version" || cmd == "--version" || cmd == "-version") {
		return nil, nil
	}
	// command is ./turbo $subcommand --help
	if len(inputFlags) == 1 && (inputFlags[0] == "help" || inputFlags[0] == "--help" || inputFlags[0] == "-help") {
		return nil, nil
	}

	cwd, err := selectCwd(args)
	if err != nil {
		return nil, err
	}
	// Precedence is flags > env > config > default
	packageJSONPath := filepath.Join(cwd, "package.json")
	rootPackageJSON, err := fs.ReadPackageJSON(packageJSONPath)
	if err != nil {
		return nil, fmt.Errorf("package.json: %w", err)
	}
	turboConfigJson, err := ReadTurboConfig(cwd, rootPackageJSON)
	if err != nil {
		return nil, err
	}
	userConfig, _ := ReadUserConfigFile()
	partialConfig, _ := ReadConfigFile(filepath.Join(".turbo", "config.json"))
	partialConfig.Token = userConfig.Token

	enverr := envconfig.Process("TURBO", partialConfig)
	if enverr != nil {
		return nil, fmt.Errorf("invalid environment variable: %w", err)
	}

	if partialConfig.Token == "" && IsCI() {
		partialConfig.Token = os.Getenv("VERCEL_ARTIFACTS_TOKEN")
		partialConfig.TeamId = os.Getenv("VERCEL_ARTIFACTS_OWNER")
	}

	app := args[0]

	// Determine our log level if we have any. First override we check if env var
	level := hclog.NoLevel
	if v := os.Getenv(EnvLogLevel); v != "" {
		level = hclog.LevelFromString(v)
		if level == hclog.NoLevel {
			return nil, fmt.Errorf("%s value %q is not a valid log level", EnvLogLevel, v)
		}
	}

	// Process arguments looking for `-v` flags to control the log level.
	// This overrides whatever the env var set.
	for _, arg := range args {
		if len(arg) != 0 && arg[0] != '-' {
			continue
		}
		switch {
		case arg == "-v":
			if level == hclog.NoLevel || level > hclog.Info {
				level = hclog.Info
			}
		case arg == "-vv":
			if level == hclog.NoLevel || level > hclog.Debug {
				level = hclog.Debug
			}
		case arg == "-vvv":
			if level == hclog.NoLevel || level > hclog.Trace {
				level = hclog.Trace
			}
		case strings.HasPrefix(arg, "--api="):
			apiUrl := arg[len("--api="):]
			if _, err := url.ParseRequestURI(apiUrl); err != nil {
				return nil, fmt.Errorf("%s is an invalid URL", apiUrl)
			}
			partialConfig.ApiUrl = apiUrl
		case strings.HasPrefix(arg, "--url="):
			loginUrl := arg[len("--url="):]
			if _, err := url.ParseRequestURI(loginUrl); err != nil {
				return nil, fmt.Errorf("%s is an invalid URL", loginUrl)
			}
			partialConfig.LoginUrl = loginUrl
		case strings.HasPrefix(arg, "--token="):
			partialConfig.Token = arg[len("--token="):]
		case strings.HasPrefix(arg, "--team="):
			partialConfig.TeamSlug = arg[len("--team="):]
		default:
			continue
		}
	}

	// Default output is nowhere unless we enable logging.
	var output io.Writer = ioutil.Discard
	color := hclog.ColorOff
	if level != hclog.NoLevel {
		output = os.Stderr
		color = hclog.AutoColor
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:   app,
		Level:  level,
		Color:  color,
		Output: output,
	})

	maxRemoteFailCount := 3
	apiClient := client.NewClient(partialConfig.ApiUrl, logger, turboVersion, partialConfig.TeamId, partialConfig.TeamSlug, uint64(maxRemoteFailCount))

	c = &Config{
		Logger:       logger,
		Token:        partialConfig.Token,
		TeamSlug:     partialConfig.TeamSlug,
		TeamId:       partialConfig.TeamId,
		ApiUrl:       partialConfig.ApiUrl,
		LoginUrl:     partialConfig.LoginUrl,
		ApiClient:    apiClient,
		TurboVersion: turboVersion,
		Cache: &CacheConfig{
			Workers: runtime.NumCPU() + 2,
			Dir:     filepath.Join("node_modules", ".cache", "turbo"),
		},
		RootPackageJSON: rootPackageJSON,
		TurboConfigJSON: turboConfigJson,
		Cwd:             cwd,
	}

	c.ApiClient.SetToken(partialConfig.Token)

	return c, nil
}

func ReadTurboConfig(rootPath string, rootPackageJSON *fs.PackageJSON) (*fs.TurboConfigJSON, error) {
	// If turbo.json exists, we use that
	// If pkg.Turbo exists, we warn about running the migration
	// Use pkg.Turbo if turbo.json doesn't exist
	// If neither exists, it's a fatal error
	turboJSONPath := filepath.Join(rootPath, "turbo.json")

	if !fs.FileExists(turboJSONPath) {
		if rootPackageJSON.LegacyTurboConfig == nil {
			// TODO: suggestion on how to create one
			return nil, fmt.Errorf("Could not find turbo.json. Follow directions at https://turborepo.org/docs/getting-started to create one")
		} else {
			log.Println("[WARNING] Turbo configuration now lives in \"turbo.json\". Migrate to turbo.json by running \"npx @turbo/codemod create-turbo-config\"")
			return rootPackageJSON.LegacyTurboConfig, nil
		}
	} else {
		turbo, err := fs.ReadTurboConfigJSON(turboJSONPath)
		if err != nil {
			return nil, fmt.Errorf("turbo.json: %w", err)
		}
		if rootPackageJSON.LegacyTurboConfig != nil {
			log.Println("[WARNING] Ignoring legacy \"turbo\" key in package.json, using turbo.json instead. Consider deleting the \"turbo\" key from package.json")
			rootPackageJSON.LegacyTurboConfig = nil
		}
		return turbo, nil
	}
}

// Selects the current working directory from OS
// and overrides with the `--cwd=` input argument
func selectCwd(inputArgs []string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("invalid working directory: %w", err)
	}
	for _, arg := range inputArgs {
		if arg == "--" {
			break
		} else if strings.HasPrefix(arg, "--cwd=") {
			if len(arg[len("--cwd="):]) > 0 {
				cwd = arg[len("--cwd="):]
			}
		}
	}
	return cwd, nil
}
