package cmd

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	ec "github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/envoyconfig"
)

var faultCmd = &cobra.Command{
	Use:   "fault",
	Short: "Toggle fault injection at runtime via the Envoy admin API",
	Long: `Toggle fault injection at runtime via the Envoy admin API.

Fault injection is isolated per --target (a backend name, or "default_app"
for the fallback route) - each route has its own runtime keys, so toggling
one target never affects any other's traffic. No config reload involved -
these hit /runtime_modify on the admin port directly, so changes take
effect on the next request. Requires the layered_runtime.admin layer in
config/envoy.yaml; without it Envoy refuses with "503 No admin layer
specified".`,
}

var faultAdminURL string

func postRuntimeModify(query string) error {
	url := fmt.Sprintf("%s/runtime_modify?%s", strings.TrimRight(faultAdminURL, "/"), query)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", resp.Status, url)
	}
	return nil
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
		abortPercent, abortStatus, _, _ := ec.FaultRuntimeKeys(faultAbortTarget)
		query := fmt.Sprintf("%s=%d&%s=%d", abortPercent, faultAbortPercent, abortStatus, faultAbortStatus)
		if err := postRuntimeModify(query); err != nil {
			return err
		}
		fmt.Printf("target=%s abort_percent=%d status=%d\n", faultAbortTarget, faultAbortPercent, faultAbortStatus)
		return nil
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
		_, _, delayPercent, delayDuration := ec.FaultRuntimeKeys(faultDelayTarget)
		query := fmt.Sprintf("%s=%d&%s=%d", delayPercent, faultDelayPercent, delayDuration, faultDelayDurationMs)
		if err := postRuntimeModify(query); err != nil {
			return err
		}
		fmt.Printf("target=%s delay_percent=%d duration_ms=%d\n", faultDelayTarget, faultDelayPercent, faultDelayDurationMs)
		return nil
	},
}

var faultResetTarget string

var faultResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset one target's abort and delay back to 0%",
	RunE: func(cmd *cobra.Command, args []string) error {
		abortPercent, _, delayPercent, _ := ec.FaultRuntimeKeys(faultResetTarget)
		query := fmt.Sprintf("%s=0&%s=0", abortPercent, delayPercent)
		if err := postRuntimeModify(query); err != nil {
			return err
		}
		fmt.Printf("target=%s fault injection reset to 0%%\n", faultResetTarget)
		return nil
	},
}

func init() {
	faultCmd.PersistentFlags().StringVar(&faultAdminURL, "admin-url", envOr("ENVOY_ADMIN_URL", "http://localhost:9901"),
		"Envoy admin API base URL.")

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
