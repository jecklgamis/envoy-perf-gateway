package render

import (
	"testing"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
	ec "github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/envoyconfig"
)

func clusterByName(cds ec.CDS, name string) (ec.Cluster, bool) {
	for _, c := range cds.Resources {
		if c.Name == name {
			return c, true
		}
	}
	return ec.Cluster{}, false
}

func vhosts(lds ec.LDS) []ec.VirtualHost {
	return lds.Resources[0].FilterChains[0].Filters[0].TypedConfig.RouteConfig.VirtualHosts
}

func catchAllRoutes(lds ec.LDS) []ec.Route {
	for _, vh := range vhosts(lds) {
		if len(vh.Domains) == 1 && vh.Domains[0] == "*" {
			return vh.Routes
		}
	}
	return nil
}

func TestBuildCDSAlwaysIncludesAdminAndDefaultApp(t *testing.T) {
	cds := BuildCDS(config.Values{})
	if _, ok := clusterByName(cds, "envoy_admin"); !ok {
		t.Error("missing envoy_admin cluster")
	}
	if _, ok := clusterByName(cds, "default_app"); !ok {
		t.Error("missing default_app cluster")
	}
	if len(cds.Resources) != 2 {
		t.Errorf("got %d clusters with no backends, want exactly 2 (admin + default_app)", len(cds.Resources))
	}
}

func TestBuildCDSAddsOneClusterPerBackend(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "a", Host: "a.internal", Port: 80, ConnectTimeout: "5s"},
		{Name: "b", Host: "b.internal", Port: 80, ConnectTimeout: "5s"},
	}}
	cds := BuildCDS(v)
	if len(cds.Resources) != 4 {
		t.Fatalf("got %d clusters, want 4 (admin + default_app + 2 backends)", len(cds.Resources))
	}
	for _, name := range []string{"a", "b"} {
		if _, ok := clusterByName(cds, name); !ok {
			t.Errorf("missing cluster %q", name)
		}
	}
}

func TestBuildCDSSetsHostAndPort(t *testing.T) {
	v := config.Values{Backends: []config.Backend{{Name: "httpbin", Host: "httpbin.org", Port: 443, ConnectTimeout: "5s"}}}
	c, ok := clusterByName(BuildCDS(v), "httpbin")
	if !ok {
		t.Fatal("missing httpbin cluster")
	}
	addr := c.LoadAssignment.Endpoints[0].LBEndpoints[0].Endpoint.Address.SocketAddress
	if addr.Address != "httpbin.org" || addr.PortValue != 443 {
		t.Errorf("got %s:%d, want httpbin.org:443", addr.Address, addr.PortValue)
	}
}

func TestBuildCDSTLSSetsTransportSocketWithSNI(t *testing.T) {
	v := config.Values{Backends: []config.Backend{{Name: "httpbin", Host: "httpbin.org", Port: 443, TLS: true, ConnectTimeout: "5s"}}}
	c, _ := clusterByName(BuildCDS(v), "httpbin")
	if c.TransportSocket == nil {
		t.Fatal("expected a transport_socket for a TLS backend, got nil")
	}
	if c.TransportSocket.TypedConfig.SNI != "httpbin.org" {
		t.Errorf("got SNI %q, want httpbin.org", c.TransportSocket.TypedConfig.SNI)
	}
}

func TestBuildCDSPlaintextHasNoTransportSocket(t *testing.T) {
	v := config.Values{Backends: []config.Backend{{Name: "svc", Host: "svc.internal", Port: 80, ConnectTimeout: "5s"}}}
	c, _ := clusterByName(BuildCDS(v), "svc")
	if c.TransportSocket != nil {
		t.Errorf("expected no transport_socket for a plaintext backend, got %+v", c.TransportSocket)
	}
}

func TestBuildCDSWithoutHTTP2HasNoProtocolOptions(t *testing.T) {
	v := config.Values{Backends: []config.Backend{{Name: "svc", Host: "svc.internal", Port: 80, ConnectTimeout: "5s"}}}
	c, _ := clusterByName(BuildCDS(v), "svc")
	if c.TypedExtensionProtocolOptions != nil {
		t.Errorf("expected no typed_extension_protocol_options without --http2, got %+v", c.TypedExtensionProtocolOptions)
	}
}

func TestBuildCDSHTTP2SetsProtocolOptions(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "grpc-svc", Host: "grpc.internal", Port: 50051, HTTP2: true, ConnectTimeout: "5s"},
	}}
	c, ok := clusterByName(BuildCDS(v), "grpc-svc")
	if !ok {
		t.Fatal("missing grpc-svc cluster")
	}
	entry, ok := c.TypedExtensionProtocolOptions["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
	if !ok {
		t.Fatalf("missing HttpProtocolOptions entry: %+v", c.TypedExtensionProtocolOptions)
	}
	opts, ok := entry.(ec.HTTPProtocolOptions)
	if !ok {
		t.Fatalf("entry is %T, want ec.HTTPProtocolOptions", entry)
	}
	if opts.ExplicitHTTPConfig.HTTP2ProtocolOptions == nil {
		t.Error("expected a non-nil (even if empty) http2_protocol_options map")
	}
}

func TestBuildCDSHTTP2WithTLSSetsALPN(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "grpc-svc", Host: "grpc.internal", Port: 50051, TLS: true, HTTP2: true, ConnectTimeout: "5s"},
	}}
	c, _ := clusterByName(BuildCDS(v), "grpc-svc")
	if c.TransportSocket == nil {
		t.Fatal("expected a transport_socket for a TLS+HTTP2 backend, got nil")
	}
	alpn := c.TransportSocket.TypedConfig.AlpnProtocols
	if len(alpn) != 1 || alpn[0] != "h2" {
		t.Errorf("got alpn_protocols=%v, want [\"h2\"]", alpn)
	}
}

func TestBuildCDSTLSWithoutHTTP2HasNoALPN(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "httpbin", Host: "httpbin.org", Port: 443, TLS: true, ConnectTimeout: "5s"},
	}}
	c, _ := clusterByName(BuildCDS(v), "httpbin")
	if len(c.TransportSocket.TypedConfig.AlpnProtocols) != 0 {
		t.Errorf("got alpn_protocols=%v for a plain TLS backend, want none", c.TransportSocket.TypedConfig.AlpnProtocols)
	}
}

func TestBuildLDSRoutePrefixRewritesToSlash(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "httpbin", RoutePrefix: "/httpbin/", Timeout: "15s"},
	}}
	routes := catchAllRoutes(BuildLDS(v))
	if len(routes) != 2 { // httpbin's route + the trailing default_app fallback
		t.Fatalf("got %d catch-all routes, want 2", len(routes))
	}
	r := routes[0]
	if r.Match.Prefix != "/httpbin/" || r.Route.Cluster != "httpbin" || r.Route.PrefixRewrite != "/" {
		t.Errorf("got %+v", r)
	}
}

func TestBuildLDSCatchAllAlwaysEndsWithDefaultAppFallback(t *testing.T) {
	routes := catchAllRoutes(BuildLDS(config.Values{}))
	if len(routes) != 1 {
		t.Fatalf("got %d routes with no backends, want 1 (the fallback)", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Match.Prefix != "/" || last.Route.Cluster != "default_app" {
		t.Errorf("got %+v, want the default_app fallback route", last)
	}
}

func TestBuildLDSNoRouteBackendGetsClusterOnly(t *testing.T) {
	v := config.Values{Backends: []config.Backend{{Name: "internal-only"}}}
	lds := BuildLDS(v)
	for _, r := range catchAllRoutes(lds) {
		if r.Route.Cluster == "internal-only" {
			t.Fatalf("backend with neither --domain nor --route-prefix must not get a route: %+v", r)
		}
	}
	for _, vh := range vhosts(lds) {
		if vh.Name == "internal-only" {
			t.Fatalf("backend with neither --domain nor --route-prefix must not get its own virtual host")
		}
	}
}

func TestBuildLDSDomainOnlyMatchesEverythingWithNoRewrite(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "svc-a", Domain: "frontend-a.test.local", Timeout: "15s"},
	}}
	lds := BuildLDS(v)
	var found *ec.VirtualHost
	all := vhosts(lds)
	for i, vh := range all {
		if len(vh.Domains) == 1 && vh.Domains[0] == "frontend-a.test.local" {
			found = &all[i]
		}
	}
	if found == nil {
		t.Fatal("missing virtual host for frontend-a.test.local")
	}
	r := found.Routes[0]
	if r.Match.Prefix != "/" || r.Route.PrefixRewrite != "" {
		t.Errorf("domain-only route should match \"/\" with no rewrite, got %+v", r)
	}
}

func TestBuildLDSDomainPlusPrefixRewrites(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "svc-b", Domain: "frontend-b.test.local", RoutePrefix: "/api/", Timeout: "15s"},
	}}
	lds := BuildLDS(v)
	for _, vh := range vhosts(lds) {
		if len(vh.Domains) == 1 && vh.Domains[0] == "frontend-b.test.local" {
			r := vh.Routes[0]
			if r.Match.Prefix != "/api/" || r.Route.PrefixRewrite != "/" {
				t.Errorf("got %+v, want prefix /api/ rewritten to /", r)
			}
			return
		}
	}
	t.Fatal("missing virtual host for frontend-b.test.local")
}

func TestBuildLDSHostRewritePropagatesToRoute(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "serverprobes", RoutePrefix: "/serverprobes/", HostRewrite: "www.serverprobes.com", Timeout: "15s"},
	}}
	routes := catchAllRoutes(BuildLDS(v))
	if routes[0].Route.HostRewriteLiteral != "www.serverprobes.com" {
		t.Errorf("got HostRewriteLiteral=%q, want www.serverprobes.com", routes[0].Route.HostRewriteLiteral)
	}
}

func TestBuildLDSEachRouteGetsIndependentFaultConfig(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "a", RoutePrefix: "/a/", Timeout: "15s"},
		{Name: "b", RoutePrefix: "/b/", Timeout: "15s"},
	}}
	routes := catchAllRoutes(BuildLDS(v))
	seen := map[string]bool{}
	for _, r := range routes {
		cfg, ok := r.TypedPerFilterConfig["envoy.filters.http.fault"].(ec.FaultTypedConfig)
		if !ok {
			t.Fatalf("route for %s missing typed_per_filter_config fault entry", r.Route.Cluster)
		}
		if seen[cfg.AbortPercentRuntime] {
			t.Fatalf("runtime key %s reused across routes - faults would not be isolated", cfg.AbortPercentRuntime)
		}
		seen[cfg.AbortPercentRuntime] = true
	}
}

func TestBuildLDSRegistersCompressorFilterDisabledByDefault(t *testing.T) {
	lds := BuildLDS(config.Values{})
	filters := lds.Resources[0].FilterChains[0].Filters[0].TypedConfig.HTTPFilters
	for _, f := range filters {
		if f.Name != "envoy.filters.http.compressor" {
			continue
		}
		cfg, ok := f.TypedConfig.(ec.CompressorTypedConfig)
		if !ok {
			t.Fatalf("compressor filter typed_config is %T, want ec.CompressorTypedConfig", f.TypedConfig)
		}
		if cfg.ResponseDirectionConfig.CommonConfig.Enabled.DefaultValue {
			t.Error("expected the listener-wide compressor filter to default to disabled")
		}
		return
	}
	t.Fatal("envoy.filters.http.compressor not found in http_filters - it must always be registered, even when no backend uses --compression, so a route can still enable it per-route")
}

func TestBuildLDSCompressionOffByDefaultForARoute(t *testing.T) {
	v := config.Values{Backends: []config.Backend{{Name: "httpbin", RoutePrefix: "/httpbin/", Timeout: "15s"}}}
	routes := catchAllRoutes(BuildLDS(v))
	if _, ok := routes[0].TypedPerFilterConfig["envoy.filters.http.compressor"]; ok {
		t.Error("a backend without --compression must not get a compressor override on its route")
	}
}

func TestBuildLDSCompressionEnablesPerRouteOverride(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "httpbin", RoutePrefix: "/httpbin/", Compression: "gzip", Timeout: "15s"},
	}}
	routes := catchAllRoutes(BuildLDS(v))
	entry, ok := routes[0].TypedPerFilterConfig["envoy.filters.http.compressor"]
	if !ok {
		t.Fatal("expected a compressor override on the route for a backend with --compression gzip")
	}
	cfg, ok := entry.(ec.CompressorPerRouteConfig)
	if !ok {
		t.Fatalf("per-route compressor entry is %T, want ec.CompressorPerRouteConfig (a distinct message from ec.CompressorTypedConfig - Envoy rejects the wrong one at listener-load time)", entry)
	}
	if !cfg.Overrides.ResponseDirectionConfig.DefaultValue {
		t.Error("expected the per-route override to enable compression")
	}
}

func TestBuildLDSCompressionDoesNotLeakToOtherRoutes(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "compressed", RoutePrefix: "/compressed/", Compression: "gzip", Timeout: "15s"},
		{Name: "plain", RoutePrefix: "/plain/", Timeout: "15s"},
	}}
	routes := catchAllRoutes(BuildLDS(v))
	for _, r := range routes {
		_, hasOverride := r.TypedPerFilterConfig["envoy.filters.http.compressor"]
		wantOverride := r.Route.Cluster == "compressed"
		if hasOverride != wantOverride {
			t.Errorf("route for %s: has compressor override=%v, want %v", r.Route.Cluster, hasOverride, wantOverride)
		}
	}
}

func TestBuildRuntimeOmitsUnsetDimensions(t *testing.T) {
	v := config.Values{Faults: map[string]config.FaultSpec{
		"httpbin": {AbortPercent: 100, AbortStatus: 503},
	}}
	keys := BuildRuntime(v)
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want exactly 2 (abort_percent + http_status, delay unset)", len(keys))
	}
	if keys["fault.http.abort.abort_percent.httpbin"] != "100" {
		t.Errorf("got %+v", keys)
	}
	if keys["fault.http.abort.http_status.httpbin"] != "503" {
		t.Errorf("got %+v", keys)
	}
}

func TestBuildRuntimeEmptyWithNoFaults(t *testing.T) {
	keys := BuildRuntime(config.Values{})
	if len(keys) != 0 {
		t.Errorf("got %+v, want an empty map", keys)
	}
}

func TestBuildRuntimeKeysAreIndependentPerTarget(t *testing.T) {
	v := config.Values{Faults: map[string]config.FaultSpec{
		"httpbin":     {AbortPercent: 100},
		"default_app": {AbortPercent: 50},
	}}
	keys := BuildRuntime(v)
	if keys["fault.http.abort.abort_percent.httpbin"] != "100" {
		t.Errorf("got %+v", keys)
	}
	if keys["fault.http.abort.abort_percent.default_app"] != "50" {
		t.Errorf("got %+v", keys)
	}
}

func TestRenderProducesValidYAMLForAllThreeFiles(t *testing.T) {
	v := config.Values{
		Backends: []config.Backend{{Name: "httpbin", Host: "httpbin.org", Port: 443, TLS: true, RoutePrefix: "/httpbin/", ConnectTimeout: "5s", Timeout: "15s"}},
		Faults:   map[string]config.FaultSpec{"httpbin": {AbortPercent: 10}},
	}
	cds, lds, runtime, err := Render(v)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(cds) == 0 || len(lds) == 0 || len(runtime) == 0 {
		t.Errorf("expected non-empty output for all three files, got cds=%d lds=%d runtime=%d bytes", len(cds), len(lds), len(runtime))
	}
}
