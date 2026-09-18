package cmd

import "github.com/spf13/cobra"

var renderCmd = &cobra.Command{
	Use:   "render",
	Short: "Render cds.yaml and lds.yaml from values.yaml without changing any backend",
	Long: `Render cds.yaml and lds.yaml from values.yaml without changing any
backend. The rendered directory is gitignored (generated), so this is
the step CI runs before "docker build" on a fresh checkout.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := regenerate()
		return err
	},
}

func init() {
	rootCmd.AddCommand(renderCmd)
}
