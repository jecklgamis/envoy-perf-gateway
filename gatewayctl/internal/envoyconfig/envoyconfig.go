// Package envoyconfig defines typed Go structs for the specific subset of
// Envoy's CDS/LDS YAML resource schema this gateway uses. Generating config
// from structs (via yaml.Marshal) instead of string templates means a
// malformed field can't produce mis-indented YAML - a whole bug class the
// previous Jinja2-template version had to work around by hand.
package envoyconfig

const (
	TypeCluster     = "type.googleapis.com/envoy.config.cluster.v3.Cluster"
	TypeListener    = "type.googleapis.com/envoy.config.listener.v3.Listener"
	TypeHCM         = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"
	TypeRouter      = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
	TypeFault       = "type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault"
	TypeUpstreamTLS = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext"
)

type SocketAddress struct {
	Address   string `yaml:"address"`
	PortValue int    `yaml:"port_value"`
}

type Address struct {
	SocketAddress SocketAddress `yaml:"socket_address"`
}

type Endpoint struct {
	Address Address `yaml:"address"`
}

type LBEndpoint struct {
	Endpoint Endpoint `yaml:"endpoint"`
}

type LocalityLbEndpoints struct {
	LBEndpoints []LBEndpoint `yaml:"lb_endpoints"`
}

type LoadAssignment struct {
	ClusterName string                `yaml:"cluster_name"`
	Endpoints   []LocalityLbEndpoints `yaml:"endpoints"`
}

type UpstreamTLSContext struct {
	Type string `yaml:"@type"`
	SNI  string `yaml:"sni"`
}

type TransportSocket struct {
	Name        string              `yaml:"name"`
	TypedConfig UpstreamTLSContext `yaml:"typed_config"`
}

type Cluster struct {
	Type            string           `yaml:"@type"`
	Name            string           `yaml:"name"`
	ConnectTimeout  string           `yaml:"connect_timeout"`
	ClusterType     string           `yaml:"type"`
	DNSLookupFamily string           `yaml:"dns_lookup_family,omitempty"`
	LBPolicy        string           `yaml:"lb_policy,omitempty"`
	LoadAssignment  LoadAssignment   `yaml:"load_assignment"`
	TransportSocket *TransportSocket `yaml:"transport_socket,omitempty"`
}

type CDS struct {
	Resources []Cluster `yaml:"resources"`
}

type RouteMatch struct {
	Prefix string `yaml:"prefix"`
}

type RouteAction struct {
	Cluster            string `yaml:"cluster"`
	PrefixRewrite      string `yaml:"prefix_rewrite,omitempty"`
	HostRewriteLiteral string `yaml:"host_rewrite_literal,omitempty"`
	Timeout            string `yaml:"timeout"`
}

// TypedPerFilterConfig keys are http_filters[].name values (e.g.
// "envoy.filters.http.fault") - this overrides that filter's behavior for
// just this route, letting each route have independently-toggleable fault
// injection instead of sharing the listener-wide default.
type Route struct {
	Match                RouteMatch     `yaml:"match"`
	Route                RouteAction    `yaml:"route"`
	TypedPerFilterConfig map[string]any `yaml:"typed_per_filter_config,omitempty"`
}

type VirtualHost struct {
	Name    string   `yaml:"name"`
	Domains []string `yaml:"domains"`
	Routes  []Route  `yaml:"routes"`
}

type RouteConfig struct {
	Name         string        `yaml:"name"`
	VirtualHosts []VirtualHost `yaml:"virtual_hosts"`
}

type RouterTypedConfig struct {
	Type string `yaml:"@type"`
}

type Percentage struct {
	Numerator   int    `yaml:"numerator"`
	Denominator string `yaml:"denominator"`
}

type FaultAbort struct {
	HTTPStatus int        `yaml:"http_status"`
	Percentage Percentage `yaml:"percentage"`
}

type FaultDelay struct {
	FixedDelay string     `yaml:"fixed_delay"`
	Percentage Percentage `yaml:"percentage"`
}

// FaultTypedConfig defaults to 0% (no-op) at render time; the *_runtime
// fields let it be toggled live via the admin API's /runtime_modify
// without a config reload - see gatewayctl's `fault` commands.
type FaultTypedConfig struct {
	Type                   string     `yaml:"@type"`
	Abort                  FaultAbort `yaml:"abort"`
	Delay                  FaultDelay `yaml:"delay"`
	AbortPercentRuntime    string     `yaml:"abort_percent_runtime"`
	AbortHTTPStatusRuntime string     `yaml:"abort_http_status_runtime"`
	DelayPercentRuntime    string     `yaml:"delay_percent_runtime"`
	DelayDurationRuntime   string     `yaml:"delay_duration_runtime"`
}

// TypedConfig is `any` since different filters (fault, router) have
// different typed_config shapes but share the same envelope.
type HTTPFilter struct {
	Name        string `yaml:"name"`
	TypedConfig any    `yaml:"typed_config"`
}

// FaultRuntimeKeys returns the *_runtime key names namespaced by target
// (a backend name, or "default_app" for the fallback route) - shared
// between render.go (which writes them into typed_per_filter_config) and
// gatewayctl's `fault` commands (which flip them via /runtime_modify), so
// toggling one target's fault state never touches another's.
func FaultRuntimeKeys(target string) (abortPercent, abortStatus, delayPercent, delayDuration string) {
	return "fault.http.abort.abort_percent." + target,
		"fault.http.abort.http_status." + target,
		"fault.http.delay.delay_percent." + target,
		"fault.http.delay.fixed_duration_ms." + target
}

// FaultPerRoute builds the typed_per_filter_config entry that gives target
// its own independently-toggleable fault injection, defaulting to 0% (a
// no-op) at render time.
func FaultPerRoute(target string) map[string]any {
	abortPercent, abortStatus, delayPercent, delayDuration := FaultRuntimeKeys(target)
	return map[string]any{
		"envoy.filters.http.fault": FaultTypedConfig{
			Type:                   TypeFault,
			Abort:                  FaultAbort{HTTPStatus: 503, Percentage: Percentage{Numerator: 0, Denominator: "HUNDRED"}},
			Delay:                  FaultDelay{FixedDelay: "2s", Percentage: Percentage{Numerator: 0, Denominator: "HUNDRED"}},
			AbortPercentRuntime:    abortPercent,
			AbortHTTPStatusRuntime: abortStatus,
			DelayPercentRuntime:    delayPercent,
			DelayDurationRuntime:   delayDuration,
		},
	}
}

type HCMTypedConfig struct {
	Type        string       `yaml:"@type"`
	StatPrefix  string       `yaml:"stat_prefix"`
	RouteConfig RouteConfig  `yaml:"route_config"`
	HTTPFilters []HTTPFilter `yaml:"http_filters"`
}

type NetworkFilter struct {
	Name        string         `yaml:"name"`
	TypedConfig HCMTypedConfig `yaml:"typed_config"`
}

type FilterChain struct {
	Filters []NetworkFilter `yaml:"filters"`
}

type Listener struct {
	Type         string        `yaml:"@type"`
	Name         string        `yaml:"name"`
	Address      Address       `yaml:"address"`
	FilterChains []FilterChain `yaml:"filter_chains"`
}

type LDS struct {
	Resources []Listener `yaml:"resources"`
}
