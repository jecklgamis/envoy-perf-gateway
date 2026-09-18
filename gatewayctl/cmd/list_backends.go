package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
)

var listBackendsCmd = &cobra.Command{
	Use:   "list-backends",
	Short: "List backends registered in values.yaml",
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := config.Load(valuesPath)
		if err != nil {
			return err
		}
		if len(v.Backends) == 0 {
			fmt.Println("No backends configured")
			return nil
		}
		for _, b := range v.Backends {
			var route string
			switch {
			case b.Domain != "" && b.RoutePrefix != "":
				route = b.Domain + b.RoutePrefix
			case b.Domain != "":
				route = b.Domain
			case b.RoutePrefix != "":
				route = b.RoutePrefix
			default:
				route = "(no route, cluster only)"
			}
			tls := "plaintext"
			if b.TLS {
				tls = "tls"
			}
			fmt.Printf("%-20s %s:%-6d %-10s %s\n", b.Name, b.Host, b.Port, tls, route)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listBackendsCmd)
}
