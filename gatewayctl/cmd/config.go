package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/settings"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or edit gatewayctl's settings file",
	Long: `View or edit the settings file gatewayctl reads for its default
distribution mode, config_server URL/token, and S3 bucket/prefix - so you
don't have to set CONFIG_SOURCE_KIND/CONFIG_SERVER_URL/etc. every session.
An env var always overrides the matching settings file value; --values/
--rendered-dir/--config are unaffected, this only covers push destination
settings.`,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the settings file path in use",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(configPath)
		return nil
	},
}

var configViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Print the current settings",
	RunE: func(cmd *cobra.Command, args []string) error {
		if appSettings == (settings.Settings{}) {
			fmt.Printf("No settings configured (%s does not exist or is empty)\n", configPath)
			return nil
		}
		data, err := yaml.Marshal(appSettings)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a key and save it to the settings file",
	Long: `Set a key and save it to the settings file. Recognized keys:

  mode             "http", "s3", or "" to unset (disables auto-push)
  http.server-url  config_server base URL
  http.api-token   Bearer token sent to config_server
  s3.bucket        S3 bucket name
  s3.prefix        S3 key prefix, e.g. "envoy-perf-gateway/"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]
		switch key {
		case "mode":
			if value != "" && value != "http" && value != "s3" {
				return fmt.Errorf(`mode must be "http", "s3", or "" to unset`)
			}
			appSettings.Mode = value
		case "http.server-url":
			appSettings.HTTP.ServerURL = value
		case "http.api-token":
			appSettings.HTTP.APIToken = value
		case "s3.bucket":
			appSettings.S3.Bucket = value
		case "s3.prefix":
			appSettings.S3.Prefix = value
		default:
			return fmt.Errorf("unknown key %q (want: mode, http.server-url, http.api-token, s3.bucket, s3.prefix)", key)
		}
		if err := settings.Save(configPath, appSettings); err != nil {
			return err
		}
		fmt.Printf("Saved %s = %q to %s\n", key, value, configPath)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configPathCmd, configViewCmd, configSetCmd)
	rootCmd.AddCommand(configCmd)
}
