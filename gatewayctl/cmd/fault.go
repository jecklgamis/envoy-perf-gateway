package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
)

var faultCmd = &cobra.Command{
	Use:   "fault",
	Short: "Toggle fault injection, distributed the same way as backends",
	Long: `Toggle fault injection, distributed the same way as backends.

Fault injection is isolated per --target (a backend name, or "default_app"
for the fallback route) - each route has its own runtime keys, so toggling
one target never affects any other's traffic.

Fault state lives in values.yaml's "faults" map alongside backends, and is
rendered into rendered/runtime.yaml and pushed through the same
config_server/S3 -> fetcher pipeline as cds.yaml/lds.yaml. Envoy picks it
up via its layered_runtime disk layer (config/envoy.yaml), the same
inotify-driven hot-reload used for routes and clusters. That means every
replica of the gateway converges on the same fault state - not just
whichever one happened to receive the command - and it survives restarts.
It also means a change takes effect on the fetcher's next poll
(CONFIG_POLL_INTERVAL_SECONDS, 15s by default), not instantly.`,
}

var (
	faultAbortTarget  string
	faultAbortPercent int
	faultAbortStatus  int
)

var faultAbortCmd = &cobra.Command{
	Use:   "abort",
	Short: "Return an error status for a percentage of one target's requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := config.Load(valuesPath)
		if err != nil {
			return err
		}
		if v.Faults == nil {
			v.Faults = map[string]config.FaultSpec{}
		}
		f := v.Faults[faultAbortTarget]
		f.AbortPercent = faultAbortPercent
		f.AbortStatus = faultAbortStatus
		v.Faults[faultAbortTarget] = f

		if err := config.Save(valuesPath, v); err != nil {
			return err
		}
		if _, err := regenerate(); err != nil {
			return err
		}
		fmt.Printf("target=%s abort_percent=%d status=%d\n", faultAbortTarget, faultAbortPercent, faultAbortStatus)
		return autoPushIfConfigured()
	},
}

var (
	faultDelayTarget     string
	faultDelayPercent    int
	faultDelayDurationMs int
)

var faultDelayCmd = &cobra.Command{
	Use:   "delay",
	Short: "Delay a percentage of one target's requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := config.Load(valuesPath)
		if err != nil {
			return err
		}
		if v.Faults == nil {
			v.Faults = map[string]config.FaultSpec{}
		}
		f := v.Faults[faultDelayTarget]
		f.DelayPercent = faultDelayPercent
		f.DelayDurationMs = faultDelayDurationMs
		v.Faults[faultDelayTarget] = f

		if err := config.Save(valuesPath, v); err != nil {
			return err
		}
		if _, err := regenerate(); err != nil {
			return err
		}
		fmt.Printf("target=%s delay_percent=%d duration_ms=%d\n", faultDelayTarget, faultDelayPercent, faultDelayDurationMs)
		return autoPushIfConfigured()
	},
}

var faultResetTarget string

var faultResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset one target's abort and delay back to 0%",
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := config.Load(valuesPath)
		if err != nil {
			return err
		}
		delete(v.Faults, faultResetTarget)

		if err := config.Save(valuesPath, v); err != nil {
			return err
		}
		if _, err := regenerate(); err != nil {
			return err
		}
		fmt.Printf("target=%s fault injection reset to 0%%\n", faultResetTarget)
		return autoPushIfConfigured()
	},
}

func init() {
	const targetHelp = `Backend name (as passed to add-backend --name), or "default_app" for the fallback route. Required.`

	faultAbortCmd.Flags().StringVar(&faultAbortTarget, "target", "", targetHelp)
	faultAbortCmd.Flags().IntVar(&faultAbortPercent, "percent", 0, "0-100 (required)")
	faultAbortCmd.Flags().IntVar(&faultAbortStatus, "status", 503, "HTTP status to return")
	_ = faultAbortCmd.MarkFlagRequired("target")
	_ = faultAbortCmd.MarkFlagRequired("percent")

	faultDelayCmd.Flags().StringVar(&faultDelayTarget, "target", "", targetHelp)
	faultDelayCmd.Flags().IntVar(&faultDelayPercent, "percent", 0, "0-100 (required)")
	faultDelayCmd.Flags().IntVar(&faultDelayDurationMs, "duration-ms", 0, "required")
	_ = faultDelayCmd.MarkFlagRequired("target")
	_ = faultDelayCmd.MarkFlagRequired("percent")
	_ = faultDelayCmd.MarkFlagRequired("duration-ms")

	faultResetCmd.Flags().StringVar(&faultResetTarget, "target", "", targetHelp)
	_ = faultResetCmd.MarkFlagRequired("target")

	faultCmd.AddCommand(faultAbortCmd, faultDelayCmd, faultResetCmd)
	rootCmd.AddCommand(faultCmd)
}
