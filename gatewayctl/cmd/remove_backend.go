package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
)

var rbName string

var removeBackendCmd = &cobra.Command{
	Use:   "remove-backend",
	Short: "Remove a backend and hot-reload Envoy",
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := config.Load(valuesPath)
		if err != nil {
			return err
		}

		before := len(v.Backends)
		filtered := v.Backends[:0]
		for _, b := range v.Backends {
			if b.Name != rbName {
				filtered = append(filtered, b)
			}
		}
		v.Backends = filtered

		if len(v.Backends) == before {
			fmt.Fprintf(os.Stderr, "No backend named '%s' found\n", rbName)
			os.Exit(1)
		}

		if err := config.Save(valuesPath, v); err != nil {
			return err
		}
		if _, err := regenerate(); err != nil {
			return err
		}
		fmt.Printf("Removed backend '%s'\n", rbName)
		return nil
	},
}

func init() {
	removeBackendCmd.Flags().StringVar(&rbName, "name", "", "")
	_ = removeBackendCmd.MarkFlagRequired("name")
	rootCmd.AddCommand(removeBackendCmd)
}
