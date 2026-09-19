package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
)

var (
	abName           string
	abHost           string
	abPort           int
	abTLS            bool
	abDomain         string
	abRoutePrefix    string
	abConnectTimeout string
	abTimeout        string
)

var addBackendCmd = &cobra.Command{
	Use:   "add-backend",
	Short: "Add (or replace) a backend and hot-reload Envoy - no restart",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateName("name", abName); err != nil {
			return err
		}

		v, err := config.Load(valuesPath)
		if err != nil {
			return err
		}

		filtered := v.Backends[:0]
		for _, b := range v.Backends {
			if b.Name != abName {
				filtered = append(filtered, b)
			}
		}
		v.Backends = append(filtered, config.Backend{
			Name:           abName,
			Host:           abHost,
			Port:           abPort,
			TLS:            abTLS,
			Domain:         abDomain,
			RoutePrefix:    abRoutePrefix,
			ConnectTimeout: abConnectTimeout,
			Timeout:        abTimeout,
		})

		if err := config.Save(valuesPath, v); err != nil {
			return err
		}
		if _, err := regenerate(); err != nil {
			return err
		}

		var routedVia []string
		if abDomain != "" {
			routedVia = append(routedVia, "domain "+abDomain)
		}
		if abRoutePrefix != "" {
			routedVia = append(routedVia, "prefix "+abRoutePrefix)
		}
		msg := fmt.Sprintf("Added backend '%s' -> %s:%d", abName, abHost, abPort)
		if len(routedVia) > 0 {
			msg += fmt.Sprintf(" (routed via %s)", strings.Join(routedVia, ", "))
		}
		fmt.Println(msg)

		return autoPushIfConfigured()
	},
}

func init() {
	addBackendCmd.Flags().StringVar(&abName, "name", "", "Cluster name, must be unique")
	addBackendCmd.Flags().StringVar(&abHost, "host", "", "Upstream host/IP")
	addBackendCmd.Flags().IntVar(&abPort, "port", 0, "Upstream port")
	addBackendCmd.Flags().BoolVar(&abTLS, "tls", false, "Terminate TLS to the upstream")
	addBackendCmd.Flags().StringVar(&abDomain, "domain", "",
		"Frontend Host header this backend is paired with, e.g. frontend-a.test.local. "+
			"Gets its own virtual host matched by domain, instead of a path prefix under "+
			"the catch-all one. Combine with --route-prefix to also scope by path within "+
			"that domain.")
	addBackendCmd.Flags().StringVar(&abRoutePrefix, "route-prefix", "",
		"Path prefix routed to this backend (rewritten to /). Omit (with no --domain "+
			"either) to only add the cluster without a route.")
	addBackendCmd.Flags().StringVar(&abConnectTimeout, "connect-timeout", "5s", "")
	addBackendCmd.Flags().StringVar(&abTimeout, "timeout", "15s", "")

	_ = addBackendCmd.MarkFlagRequired("name")
	_ = addBackendCmd.MarkFlagRequired("host")
	_ = addBackendCmd.MarkFlagRequired("port")

	rootCmd.AddCommand(addBackendCmd)
}
