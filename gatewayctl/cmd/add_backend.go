package cmd

import (
	"fmt"
	"os"
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
	abHostRewrite    string
	abHTTP2          bool
	abCompression    string
	abConnectTimeout string
	abTimeout        string
	abForce          bool
)

var addBackendCmd = &cobra.Command{
	Use:   "add-backend",
	Short: "Add (or replace) a backend and hot-reload Envoy - no restart",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateName("name", abName); err != nil {
			return err
		}
		if err := validatePort("port", abPort); err != nil {
			return err
		}
		if err := validateDuration("connect-timeout", abConnectTimeout); err != nil {
			return err
		}
		if err := validateDuration("timeout", abTimeout); err != nil {
			return err
		}
		if err := validateCompression("compression", abCompression); err != nil {
			return err
		}

		// A missing values.yaml with a mode configured is the classic
		// wrong-directory/wrong-machine trap: config.Load silently treats
		// it as an empty backend list, so this push would look identical
		// to a legitimate first backend, but could actually be about to
		// overwrite a remote config that already has other backends on
		// it with just this one. --force is required to proceed anyway
		// (fine for a genuinely fresh deployment, or scripted use).
		if !abForce && resolveMode() != "" {
			if _, statErr := os.Stat(valuesPath); os.IsNotExist(statErr) {
				return fmt.Errorf(
					"%s doesn't exist yet, but a mode is configured (%s) - pushing now could "+
						"silently overwrite a remote config that already has other backends on it "+
						"with just this one. Run 'gatewayctl list-backends --remote' to check first, "+
						"or pass --force if this is genuinely a fresh deployment",
					valuesPath, resolveMode())
			}
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
			HostRewrite:    abHostRewrite,
			HTTP2:          abHTTP2,
			Compression:    abCompression,
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
		if abHostRewrite != "" {
			routedVia = append(routedVia, "Host header "+abHostRewrite)
		}
		if abHTTP2 {
			routedVia = append(routedVia, "HTTP/2 upstream")
		}
		if abCompression != "" {
			routedVia = append(routedVia, abCompression+" compression")
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
	addBackendCmd.Flags().StringVar(&abHostRewrite, "host-header", "",
		"Overrides the Host header sent to the upstream, e.g. the backend's own "+
			"hostname. Needed for backends (Cloudflare-fronted ones especially) that "+
			"reject a request whose Host header doesn't match the TLS SNI/cert - "+
			"--host set instead controls the SNI at connect time, this controls what "+
			"the upstream actually sees in the request itself. Omit to pass the "+
			"gateway's own inbound Host header through unchanged.")
	addBackendCmd.Flags().BoolVar(&abHTTP2, "http2", false,
		"Speak HTTP/2 to this backend's upstream (required for gRPC). Envoy "+
			"otherwise defaults every cluster to HTTP/1.1 upstream regardless of "+
			"what the listener or client negotiated. Use --domain, not "+
			"--route-prefix, for a gRPC backend - path rewriting breaks gRPC's "+
			"fixed /package.Service/Method paths. If deployed behind an Ingress "+
			"(the Helm charts), that hop also needs its own HTTP/2 or gRPC "+
			"backend-protocol annotation - this flag only covers gatewayctl's "+
			"own upstream connection.")
	addBackendCmd.Flags().StringVar(&abCompression, "compression", "",
		"Enable response compression for this backend's route. Only \"gzip\" is "+
			"supported today. Off by default and opt-in per backend, not "+
			"gateway-wide - compression changes latency/CPU characteristics that "+
			"would otherwise silently affect a load test nobody asked to have "+
			"compressed.")
	addBackendCmd.Flags().StringVar(&abConnectTimeout, "connect-timeout", "5s", "")
	addBackendCmd.Flags().StringVar(&abTimeout, "timeout", "15s", "")
	addBackendCmd.Flags().BoolVar(&abForce, "force", false,
		"Skip the missing-values.yaml safety check (see its error message for why it exists).")

	_ = addBackendCmd.MarkFlagRequired("name")
	_ = addBackendCmd.MarkFlagRequired("host")
	_ = addBackendCmd.MarkFlagRequired("port")

	rootCmd.AddCommand(addBackendCmd)
}
