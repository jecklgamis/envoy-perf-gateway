package cmd

import (
	"fmt"

	"gopkg.in/yaml.v3"

	ec "github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/envoyconfig"
)

// remoteRouteInfo is what findRemoteRoute recovers about a cluster's route
// (if it has one) from lds.yaml - enough to reconstruct the same fields
// add-backend's --domain/--route-prefix/--host-header flags set.
type remoteRouteInfo struct {
	Domain      string
	RoutePrefix string
	HostRewrite string
	Timeout     string
}

// findRemoteRoute finds the route (if any) pointed at clusterName across
// every virtual host in lds. ok is false for a cluster-only backend (no
// route at all) - the caller decides what that means for its purpose.
func findRemoteRoute(lds ec.LDS, clusterName string) (remoteRouteInfo, bool) {
	for _, listener := range lds.Resources {
		for _, fc := range listener.FilterChains {
			for _, f := range fc.Filters {
				for _, vh := range f.TypedConfig.RouteConfig.VirtualHosts {
					catchAll := len(vh.Domains) == 1 && vh.Domains[0] == "*"
					for _, r := range vh.Routes {
						if r.Route.Cluster != clusterName {
							continue
						}
						info := remoteRouteInfo{
							HostRewrite: r.Route.HostRewriteLiteral,
							Timeout:     r.Route.Timeout,
						}
						if catchAll {
							// render.go always sets prefix_rewrite for a
							// catch-all route, so Match.Prefix here is
							// always the literal --route-prefix value.
							info.RoutePrefix = r.Match.Prefix
							return info, true
						}
						info.Domain = vh.Domains[0]
						// For a domain vhost, render.go only sets
						// prefix_rewrite when --route-prefix was
						// explicitly given - a domain-only backend gets
						// Match.Prefix "/" with no rewrite. That's the
						// one case an empty RoutePrefix here is
						// ambiguous with an explicit "/", so key off
						// PrefixRewrite instead of the prefix value.
						if r.Route.PrefixRewrite != "" {
							info.RoutePrefix = r.Match.Prefix
						}
						return info, true
					}
				}
			}
		}
	}
	return remoteRouteInfo{}, false
}

// fetchRemoteCDSAndLDS fetches and decodes the remote cds.yaml/lds.yaml.
// found is false if either is missing (nothing pushed yet).
func fetchRemoteCDSAndLDS() (cds ec.CDS, lds ec.LDS, found bool, err error) {
	cdsBody, found, err := fetchRemoteFile("cds.yaml")
	if err != nil || !found {
		return cds, lds, found, err
	}
	ldsBody, found, err := fetchRemoteFile("lds.yaml")
	if err != nil || !found {
		return cds, lds, found, err
	}
	if err := yaml.Unmarshal(cdsBody, &cds); err != nil {
		return cds, lds, false, fmt.Errorf("decoding remote cds.yaml: %w", err)
	}
	if err := yaml.Unmarshal(ldsBody, &lds); err != nil {
		return cds, lds, false, fmt.Errorf("decoding remote lds.yaml: %w", err)
	}
	return cds, lds, true, nil
}
