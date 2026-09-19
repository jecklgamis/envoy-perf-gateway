package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
	ec "github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/envoyconfig"
)

var lbRemote bool

var listBackendsCmd = &cobra.Command{
	Use:   "list-backends",
	Short: "List backends registered in values.yaml, or actually live with --remote",
	RunE: func(cmd *cobra.Command, args []string) error {
		if lbRemote {
			return listRemoteBackends()
		}
		return listLocalBackends()
	},
}

func listLocalBackends() error {
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
}

// listRemoteBackends fetches the config server's/S3's current cds.yaml and
// lds.yaml - the same files the in-container fetcher polls - and prints
// the same table listLocalBackends does, but reconstructed from what's
// actually live rather than local values.yaml. The two can disagree (a
// push that failed silently, or values.yaml here not being the one that
// was last used to push), which is exactly the drift this is for.
func listRemoteBackends() error {
	cdsBody, found, err := fetchRemoteFile("cds.yaml")
	if err != nil {
		return err
	}
	if !found {
		fmt.Println("No cds.yaml on the remote source yet (nothing pushed)")
		return nil
	}
	ldsBody, found, err := fetchRemoteFile("lds.yaml")
	if err != nil {
		return err
	}
	if !found {
		fmt.Println("No lds.yaml on the remote source yet (nothing pushed)")
		return nil
	}

	var cds ec.CDS
	if err := yaml.Unmarshal(cdsBody, &cds); err != nil {
		return fmt.Errorf("decoding remote cds.yaml: %w", err)
	}
	var lds ec.LDS
	if err := yaml.Unmarshal(ldsBody, &lds); err != nil {
		return fmt.Errorf("decoding remote lds.yaml: %w", err)
	}

	printed := false
	for _, c := range cds.Resources {
		if c.Name == "envoy_admin" || c.Name == "default_app" {
			continue
		}
		printed = true
		host, port := "?", 0
		if len(c.LoadAssignment.Endpoints) > 0 && len(c.LoadAssignment.Endpoints[0].LBEndpoints) > 0 {
			addr := c.LoadAssignment.Endpoints[0].LBEndpoints[0].Endpoint.Address.SocketAddress
			host, port = addr.Address, addr.PortValue
		}
		tls := "plaintext"
		if c.TransportSocket != nil {
			tls = "tls"
		}
		fmt.Printf("%-20s %s:%-6d %-10s %s\n", c.Name, host, port, tls, remoteRouteDescription(lds, c.Name))
	}
	if !printed {
		fmt.Println("No backends configured")
	}
	return nil
}

// remoteRouteDescription finds the route (if any) pointed at clusterName
// across every virtual host in lds and formats it the same way
// listLocalBackends formats a Backend's domain/route-prefix.
func remoteRouteDescription(lds ec.LDS, clusterName string) string {
	for _, listener := range lds.Resources {
		for _, fc := range listener.FilterChains {
			for _, f := range fc.Filters {
				for _, vh := range f.TypedConfig.RouteConfig.VirtualHosts {
					catchAll := len(vh.Domains) == 1 && vh.Domains[0] == "*"
					for _, r := range vh.Routes {
						if r.Route.Cluster != clusterName {
							continue
						}
						prefix := r.Match.Prefix
						if catchAll {
							return prefix
						}
						domain := strings.Join(vh.Domains, ",")
						if prefix != "" && prefix != "/" {
							return domain + prefix
						}
						return domain
					}
				}
			}
		}
	}
	return "(no route, cluster only)"
}

func init() {
	listBackendsCmd.Flags().BoolVar(&lbRemote, "remote", false,
		"Fetch cds.yaml/lds.yaml from the config server or S3 and list what's actually live, instead of reading local values.yaml.")
	rootCmd.AddCommand(listBackendsCmd)
}
