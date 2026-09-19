package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/atomicwrite"
	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/render"
	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/settings"
)

// validNameRE restricts backend/fault-target names to a safe charset.
// These names end up as Envoy runtime keys that the fetcher (running as
// root inside the gateway container) turns into filesystem paths via
// expandRuntimeLayer - without this, a name containing "/" or ".."
// segments could write outside its intended directory.
var validNameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func validateName(flag, value string) error {
	if !validNameRE.MatchString(value) {
		return fmt.Errorf("--%s %q: only letters, digits, '-', and '_' are allowed", flag, value)
	}
	return nil
}

var (
	valuesPath  string
	renderedDir string
	configPath  string

	// appSettings is loaded in rootCmd's PersistentPreRunE, once flags are
	// parsed - not at package-init time, so --config/GATEWAYCTL_CONFIG can
	// actually pick which file gets read.
	appSettings settings.Settings
)

var rootCmd = &cobra.Command{
	Use:   "gatewayctl",
	Short: "envoy-perf-gateway control CLI",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		s, err := settings.Load(configPath)
		if err != nil {
			return fmt.Errorf("loading %s: %w", configPath, err)
		}
		appSettings = s
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	defaultValues := filepath.Join(cwd, "config", "values.yaml")
	defaultRendered := filepath.Join(cwd, "rendered")

	rootCmd.PersistentFlags().StringVar(&valuesPath, "values", envOr("GATEWAYCTL_VALUES", defaultValues),
		"Path to values.yaml. Defaults to config/values.yaml in the current working directory.")
	rootCmd.PersistentFlags().StringVar(&renderedDir, "rendered-dir", envOr("GATEWAYCTL_RENDERED_DIR", defaultRendered),
		"Directory to write rendered cds.yaml/lds.yaml/runtime.yaml into.")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", envOr("GATEWAYCTL_CONFIG", settings.DefaultPath()),
		"Path to gatewayctl's settings file (mode, config_server URL/token, S3 bucket/prefix).")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// regenerate loads values.yaml, renders cds.yaml/lds.yaml/runtime.yaml, and atomically
// writes both into renderedDir. Every mutating command calls this so the
// rendered output is always in sync with values.yaml on disk.
func regenerate() (config.Values, error) {
	v, err := config.Load(valuesPath)
	if err != nil {
		return v, fmt.Errorf("loading %s: %w", valuesPath, err)
	}
	if err := os.MkdirAll(renderedDir, 0o755); err != nil {
		return v, err
	}
	cds, lds, runtime, err := render.Render(v)
	if err != nil {
		return v, err
	}
	if err := atomicwrite.Write(filepath.Join(renderedDir, "cds.yaml"), cds); err != nil {
		return v, err
	}
	if err := atomicwrite.Write(filepath.Join(renderedDir, "lds.yaml"), lds); err != nil {
		return v, err
	}
	if err := atomicwrite.Write(filepath.Join(renderedDir, "runtime.yaml"), runtime); err != nil {
		return v, err
	}
	fmt.Printf("Regenerated %s/cds.yaml, lds.yaml, and runtime.yaml\n", renderedDir)
	return v, nil
}

// Each resolver applies env var > settings file > hard-coded default, in
// that order - an env var always wins so CI/container-style overrides
// still work without touching the settings file.

func resolveMode() string {
	if v := os.Getenv("CONFIG_SOURCE_KIND"); v != "" {
		return v
	}
	return appSettings.Mode
}

func resolveServerURL() string {
	if v := os.Getenv("CONFIG_SERVER_URL"); v != "" {
		return v
	}
	if appSettings.HTTP.ServerURL != "" {
		return appSettings.HTTP.ServerURL
	}
	return "http://localhost:8090"
}

func resolveAPIToken() string {
	if v := os.Getenv("CONFIG_SERVER_API_TOKEN"); v != "" {
		return v
	}
	return appSettings.HTTP.APIToken
}

func resolveS3Bucket() string {
	if v := os.Getenv("CONFIG_S3_BUCKET"); v != "" {
		return v
	}
	return appSettings.S3.Bucket
}

func resolveS3Prefix() string {
	if v := os.Getenv("CONFIG_S3_PREFIX"); v != "" {
		return v
	}
	return appSettings.S3.Prefix
}

// autoPushIfConfigured pushes the already-rendered cds.yaml/lds.yaml/runtime.yaml when a
// mode is configured, via CONFIG_SOURCE_KIND or the settings file's `mode`
// - the same signal the fetcher (running inside the container) uses to
// decide where to read config from. This lets add-backend/remove-backend
// double as "and distribute it" without a separate push-http/push-s3 call,
// while staying a no-op (today's behavior, unchanged) when no mode is set
// anywhere.
func autoPushIfConfigured() error {
	switch mode := resolveMode(); mode {
	case "":
		return nil
	case "http":
		return pushHTTPFiles(resolveServerURL(), resolveAPIToken())
	case "s3":
		bucket := resolveS3Bucket()
		if bucket == "" {
			return fmt.Errorf("mode=s3 requires CONFIG_S3_BUCKET or settings file s3.bucket to be set")
		}
		return pushS3Files(bucket, resolveS3Prefix())
	default:
		return fmt.Errorf("unsupported mode: %s (want \"http\" or \"s3\")", mode)
	}
}
