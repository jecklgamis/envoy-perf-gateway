package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
)

var gvForce bool

var generateValuesCmd = &cobra.Command{
	Use:   "generate-values",
	Short: "Reconstruct values.yaml from what's actually live on the config server or S3",
	Long: `Reconstruct values.yaml from what's actually live on the config server or
S3, the same way "kubectl get -o yaml" exports a live object's manifest.

Every field values.yaml has (host, port, tls, domain, route-prefix,
host-header, timeouts, faults) is fully recoverable from cds.yaml/lds.yaml/
runtime.yaml, so this isn't a lossy approximation - it's the same backend
list and fault state you'd get by re-running the add-backend/fault
commands that produced it.

Use this to sync local state before making further changes (the same
shape as "git pull" before "git push"), instead of trusting whatever's
already sitting in local values.yaml - which may be stale, or may not be
the copy that was last used to push at all.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !gvForce {
			if _, err := os.Stat(valuesPath); err == nil {
				return fmt.Errorf(
					"%s already exists - generate-values would overwrite it with whatever's "+
						"actually live, discarding any local-only changes not yet pushed. "+
						"Pass --force to proceed", valuesPath)
			}
		}

		cds, lds, found, err := fetchRemoteCDSAndLDS()
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("no config on the remote source yet (nothing pushed)")
		}

		v := config.Values{Backends: []config.Backend{}}
		for _, c := range cds.Resources {
			if c.Name == "envoy_admin" || c.Name == "default_app" {
				continue
			}
			b := config.Backend{
				Name:           c.Name,
				TLS:            c.TransportSocket != nil,
				ConnectTimeout: c.ConnectTimeout,
				Timeout:        "15s",
			}
			if len(c.LoadAssignment.Endpoints) > 0 && len(c.LoadAssignment.Endpoints[0].LBEndpoints) > 0 {
				addr := c.LoadAssignment.Endpoints[0].LBEndpoints[0].Endpoint.Address.SocketAddress
				b.Host, b.Port = addr.Address, addr.PortValue
			}
			if info, ok := findRemoteRoute(lds, c.Name); ok {
				b.Domain = info.Domain
				b.RoutePrefix = info.RoutePrefix
				b.HostRewrite = info.HostRewrite
				if info.Timeout != "" {
					b.Timeout = info.Timeout
				}
			}
			v.Backends = append(v.Backends, b)
		}

		if runtimeBody, found, err := fetchRemoteFile("runtime.yaml"); err != nil {
			return err
		} else if found {
			faults, err := parseRemoteFaults(runtimeBody)
			if err != nil {
				return err
			}
			if len(faults) > 0 {
				v.Faults = faults
			}
		}

		if err := config.Save(valuesPath, v); err != nil {
			return err
		}
		fmt.Printf("Wrote %s: %d backend(s), %d fault override(s)\n", valuesPath, len(v.Backends), len(v.Faults))
		return nil
	},
}

// parseRemoteFaults reverses envoyconfig.FaultRuntimeKeys: runtime.yaml is
// a flat map of Envoy runtime key -> string value, each key namespaced by
// target, and this recovers the per-target config.FaultSpec that produced
// it.
func parseRemoteFaults(body []byte) (map[string]config.FaultSpec, error) {
	var entries map[string]string
	if err := yaml.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("decoding remote runtime.yaml: %w", err)
	}

	const (
		abortPercentPrefix  = "fault.http.abort.abort_percent."
		abortStatusPrefix   = "fault.http.abort.http_status."
		delayPercentPrefix  = "fault.http.delay.delay_percent."
		delayDurationPrefix = "fault.http.delay.fixed_duration_ms."
	)

	faults := map[string]config.FaultSpec{}
	for key, value := range entries {
		var target string
		var apply func(*config.FaultSpec, int)
		switch {
		case strings.HasPrefix(key, abortPercentPrefix):
			target = strings.TrimPrefix(key, abortPercentPrefix)
			apply = func(f *config.FaultSpec, n int) { f.AbortPercent = n }
		case strings.HasPrefix(key, abortStatusPrefix):
			target = strings.TrimPrefix(key, abortStatusPrefix)
			apply = func(f *config.FaultSpec, n int) { f.AbortStatus = n }
		case strings.HasPrefix(key, delayPercentPrefix):
			target = strings.TrimPrefix(key, delayPercentPrefix)
			apply = func(f *config.FaultSpec, n int) { f.DelayPercent = n }
		case strings.HasPrefix(key, delayDurationPrefix):
			target = strings.TrimPrefix(key, delayDurationPrefix)
			apply = func(f *config.FaultSpec, n int) { f.DelayDurationMs = n }
		default:
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("runtime key %s: value %q is not an integer: %w", key, value, err)
		}
		f := faults[target]
		apply(&f, n)
		faults[target] = f
	}
	return faults, nil
}

func init() {
	generateValuesCmd.Flags().BoolVar(&gvForce, "force", false,
		"Overwrite values.yaml if it already exists.")
	rootCmd.AddCommand(generateValuesCmd)
}
