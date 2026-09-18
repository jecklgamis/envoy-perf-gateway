// Package render builds the CDS/LDS Envoy resources from values.yaml and
// marshals them to YAML.
package render

import (
	"gopkg.in/yaml.v3"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
	ec "github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/envoyconfig"
)

func singleEndpointLoadAssignment(clusterName, address string, port int) ec.LoadAssignment {
	return ec.LoadAssignment{
		ClusterName: clusterName,
		Endpoints: []ec.LocalityLbEndpoints{
			{LBEndpoints: []ec.LBEndpoint{
				{Endpoint: ec.Endpoint{Address: ec.Address{SocketAddress: ec.SocketAddress{
					Address: address, PortValue: port,
				}}}},
			}},
		},
	}
}

// BuildCDS returns envoy_admin, default_app, and one cluster per backend.
func BuildCDS(v config.Values) ec.CDS {
	clusters := []ec.Cluster{
		{
			Type:           ec.TypeCluster,
			Name:           "envoy_admin",
			ConnectTimeout: "0.25s",
			ClusterType:    "STATIC",
			LoadAssignment: singleEndpointLoadAssignment("envoy_admin", "127.0.0.1", 9901),
		},
		{
			Type:            ec.TypeCluster,
			Name:            "default_app",
			ConnectTimeout:  "0.25s",
			ClusterType:     "LOGICAL_DNS",
			DNSLookupFamily: "V4_ONLY",
			LBPolicy:        "ROUND_ROBIN",
			LoadAssignment:  singleEndpointLoadAssignment("default_app", "127.0.0.1", 5050),
		},
	}

	for _, b := range v.Backends {
		c := ec.Cluster{
			Type:            ec.TypeCluster,
			Name:            b.Name,
			ConnectTimeout:  b.ConnectTimeout,
			ClusterType:     "LOGICAL_DNS",
			DNSLookupFamily: "V4_ONLY",
			LBPolicy:        "ROUND_ROBIN",
			LoadAssignment:  singleEndpointLoadAssignment(b.Name, b.Host, b.Port),
		}
		if b.TLS {
			c.TransportSocket = &ec.TransportSocket{
				Name: "envoy.transport_sockets.tls",
				TypedConfig: ec.UpstreamTLSContext{
					Type: ec.TypeUpstreamTLS,
					SNI:  b.Host,
				},
			}
		}
		clusters = append(clusters, c)
	}

	return ec.CDS{Resources: clusters}
}

// BuildLDS returns a single http_listener. Backends with --domain get their
// own virtual host matched by Host header; everything else (route-prefix
// only, or no route at all) shares the catch-all "*" virtual host, which
// always ends with a "/" -> default_app fallback route.
func BuildLDS(v config.Values) ec.LDS {
	var domainVHosts []ec.VirtualHost
	var catchAllRoutes []ec.Route

	for _, b := range v.Backends {
		switch {
		case b.Domain != "":
			prefix := "/"
			if b.RoutePrefix != "" {
				prefix = b.RoutePrefix
			}
			action := ec.RouteAction{Cluster: b.Name, Timeout: b.Timeout}
			if b.RoutePrefix != "" {
				action.PrefixRewrite = "/"
			}
			domainVHosts = append(domainVHosts, ec.VirtualHost{
				Name:    b.Name,
				Domains: []string{b.Domain},
				Routes: []ec.Route{
					{Match: ec.RouteMatch{Prefix: prefix}, Route: action, TypedPerFilterConfig: ec.FaultPerRoute(b.Name)},
				},
			})
		case b.RoutePrefix != "":
			catchAllRoutes = append(catchAllRoutes, ec.Route{
				Match:                ec.RouteMatch{Prefix: b.RoutePrefix},
				Route:                ec.RouteAction{Cluster: b.Name, PrefixRewrite: "/", Timeout: b.Timeout},
				TypedPerFilterConfig: ec.FaultPerRoute(b.Name),
			})
		}
	}

	catchAllRoutes = append(catchAllRoutes, ec.Route{
		Match:                ec.RouteMatch{Prefix: "/"},
		Route:                ec.RouteAction{Cluster: "default_app", Timeout: "15s"},
		TypedPerFilterConfig: ec.FaultPerRoute("default_app"),
	})

	virtualHosts := append(domainVHosts, ec.VirtualHost{
		Name:    "backend",
		Domains: []string{"*"},
		Routes:  catchAllRoutes,
	})

	listener := ec.Listener{
		Type: ec.TypeListener,
		Name: "http_listener",
		Address: ec.Address{SocketAddress: ec.SocketAddress{
			Address: "0.0.0.0", PortValue: 8080,
		}},
		FilterChains: []ec.FilterChain{
			{Filters: []ec.NetworkFilter{
				{
					Name: "envoy.filters.network.http_connection_manager",
					TypedConfig: ec.HCMTypedConfig{
						Type:       ec.TypeHCM,
						StatPrefix: "ingress_http",
						RouteConfig: ec.RouteConfig{
							Name:         "local_route",
							VirtualHosts: virtualHosts,
						},
						HTTPFilters: []ec.HTTPFilter{
							{Name: "envoy.filters.http.fault", TypedConfig: ec.FaultTypedConfig{
								Type:                   ec.TypeFault,
								Abort:                  ec.FaultAbort{HTTPStatus: 503, Percentage: ec.Percentage{Numerator: 0, Denominator: "HUNDRED"}},
								Delay:                  ec.FaultDelay{FixedDelay: "2s", Percentage: ec.Percentage{Numerator: 0, Denominator: "HUNDRED"}},
								AbortPercentRuntime:    "fault.http.abort.abort_percent",
								AbortHTTPStatusRuntime: "fault.http.abort.http_status",
								DelayPercentRuntime:    "fault.http.delay.delay_percent",
								DelayDurationRuntime:   "fault.http.delay.fixed_duration_ms",
							}},
							{Name: "envoy.filters.http.router", TypedConfig: ec.RouterTypedConfig{Type: ec.TypeRouter}},
						},
					},
				},
			}},
		},
	}

	return ec.LDS{Resources: []ec.Listener{listener}}
}

// Render returns the marshaled cds.yaml and lds.yaml bytes for v.
func Render(v config.Values) (cds []byte, lds []byte, err error) {
	cds, err = yaml.Marshal(BuildCDS(v))
	if err != nil {
		return nil, nil, err
	}
	lds, err = yaml.Marshal(BuildLDS(v))
	if err != nil {
		return nil, nil, err
	}
	return cds, lds, nil
}
