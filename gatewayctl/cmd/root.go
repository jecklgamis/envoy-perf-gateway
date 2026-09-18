package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/atomicwrite"
	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/render"
)

var (
	valuesPath  string
	renderedDir string
)

var rootCmd = &cobra.Command{
	Use:   "gatewayctl",
	Short: "envoy-perf-gateway control CLI",
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
	defaultValues := filepath.Join(cwd, "values.yaml")
	defaultRendered := filepath.Join(cwd, "rendered")

	rootCmd.PersistentFlags().StringVar(&valuesPath, "values", envOr("GATEWAYCTL_VALUES", defaultValues),
		"Path to values.yaml. Defaults to values.yaml in the current working directory.")
	rootCmd.PersistentFlags().StringVar(&renderedDir, "rendered-dir", envOr("GATEWAYCTL_RENDERED_DIR", defaultRendered),
		"Directory to write rendered cds.yaml/lds.yaml into.")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// regenerate loads values.yaml, renders cds.yaml/lds.yaml, and atomically
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
	cds, lds, err := render.Render(v)
	if err != nil {
		return v, err
	}
	if err := atomicwrite.Write(filepath.Join(renderedDir, "cds.yaml"), cds); err != nil {
		return v, err
	}
	if err := atomicwrite.Write(filepath.Join(renderedDir, "lds.yaml"), lds); err != nil {
		return v, err
	}
	fmt.Printf("Regenerated %s/cds.yaml and lds.yaml\n", renderedDir)
	return v, nil
}
