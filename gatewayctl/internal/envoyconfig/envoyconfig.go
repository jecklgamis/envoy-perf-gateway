// Package envoyconfig defines typed Go structs for the specific subset of
// Envoy's CDS/LDS YAML resource schema this gateway uses. Generating config
// from structs (via yaml.Marshal) instead of string templates means a
// malformed field can't produce mis-indented YAML - a whole bug class the
// previous Jinja2-template version had to work around by hand.
package envoyconfig

const (
	TypeCluster             = "type.googleapis.com/envoy.config.cluster.v3.Cluster"
	TypeListener            = "type.googleapis.com/envoy.config.listener.v3.Listener"
	TypeHCM                 = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"
	TypeRouter              = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
	TypeFault               = "type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault"
	TypeUpstreamTLS         = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext"
	TypeHTTPProtocolOptions = "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions"
	TypeCompressor          = "type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor"
	TypeCompressorPerRoute  = "type.googleapis.com/envoy.extensions.filters.http.compressor.v3.CompressorPerRoute"
	TypeGzipCompressor      = "type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip"
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
	// AlpnProtocols advertises protocol preference during the TLS
	// handshake - set alongside TypedExtensionProtocolOptions below when a
	// backend is HTTP/2, since some servers pick their behavior based on
	// the negotiated ALPN result rather than just accepting whatever
	// codec the client speaks first.
	AlpnProtocols []string `yaml:"alpn_protocols,omitempty"`
}

type TransportSocket struct {
	Name        string             `yaml:"name"`
	TypedConfig UpstreamTLSContext `yaml:"typed_config"`
}

// ExplicitHTTPConfig and HTTPProtocolOptions build the
// typed_extension_protocol_options entry that tells Envoy to speak HTTP/2
// to a cluster's upstream. Without this, Envoy defaults every cluster to
// HTTP/1.1 upstream regardless of what the downstream listener or client
// negotiated - this is what actually gRPC/HTTP2-enables a backend, not
// anything on the listener side.
type ExplicitHTTPConfig struct {
	HTTP2ProtocolOptions map[string]any `yaml:"http2_protocol_options"`
}

type HTTPProtocolOptions struct {
	Type               string             `yaml:"@type"`
	ExplicitHTTPConfig ExplicitHTTPConfig `yaml:"explicit_http_config"`
}

type Cluster struct {
	Type                          string           `yaml:"@type"`
	Name                          string           `yaml:"name"`
	ConnectTimeout                string           `yaml:"connect_timeout"`
	ClusterType                   string           `yaml:"type"`
	DNSLookupFamily               string           `yaml:"dns_lookup_family,omitempty"`
	LBPolicy                      string           `yaml:"lb_policy,omitempty"`
	LoadAssignment                LoadAssignment   `yaml:"load_assignment"`
	TransportSocket               *TransportSocket `yaml:"transport_socket,omitempty"`
	TypedExtensionProtocolOptions map[string]any   `yaml:"typed_extension_protocol_options,omitempty"`
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

// compressibleContentTypes is the set of response Content-Types worth
// gzipping - text/JSON payloads compress well; skipping everything else
// avoids wasting CPU re-compressing content (images, video, already-
// compressed archives) that wouldn't shrink further.
var compressibleContentTypes = []string{
	"application/json",
	"application/javascript",
	"application/xml",
	"text/plain",
	"text/html",
	"text/css",
	"text/javascript",
}

type GzipCompressorLibraryConfig struct {
	Type string `yaml:"@type"`
}

type CompressorLibrary struct {
	Name        string `yaml:"name"`
	TypedConfig any    `yaml:"typed_config"`
}

type RuntimeFeatureFlag struct {
	DefaultValue bool `yaml:"default_value"`
}

type CompressorCommonDirectionConfig struct {
	Enabled RuntimeFeatureFlag `yaml:"enabled"`
}

type CompressorResponseDirectionConfig struct {
	CommonConfig     CompressorCommonDirectionConfig `yaml:"common_config"`
	ContentType      []string                        `yaml:"content_type,omitempty"`
	MinContentLength int                             `yaml:"min_content_length,omitempty"`
}

type CompressorTypedConfig struct {
	Type                    string                            `yaml:"@type"`
	CompressorLibrary       CompressorLibrary                 `yaml:"compressor_library"`
	ResponseDirectionConfig CompressorResponseDirectionConfig `yaml:"response_direction_config"`
}

// Compressor builds the envoy.filters.http.compressor filter's own
// top-level config - registered once in http_filters, default_value=false
// so compression is off listener-wide unless a route overrides it.
func Compressor() CompressorTypedConfig {
	return CompressorTypedConfig{
		Type: TypeCompressor,
		CompressorLibrary: CompressorLibrary{
			Name:        "gzip",
			TypedConfig: GzipCompressorLibraryConfig{Type: TypeGzipCompressor},
		},
		ResponseDirectionConfig: CompressorResponseDirectionConfig{
			CommonConfig:     CompressorCommonDirectionConfig{Enabled: RuntimeFeatureFlag{DefaultValue: false}},
			ContentType:      compressibleContentTypes,
			MinContentLength: 860,
		},
	}
}

// CompressorPerRouteOverrides and CompressorPerRouteConfig are a
// *different* message from CompressorTypedConfig above -
// envoy.extensions.filters.http.compressor.v3.CompressorPerRoute, not
// another Compressor. Reusing the top-level Compressor message here (an
// earlier version of this code did) fails at listener-load time with
// "Unable to unpack as ...CompressorPerRoute" - confirmed against a real
// Envoy instance via its /config_dump error_state, not just inferred from
// docs. A route's typed_per_filter_config for the compressor filter can
// only flip the enabled flag via this dedicated per-route message.
type CompressorPerRouteOverrides struct {
	ResponseDirectionConfig RuntimeFeatureFlag `yaml:"response_direction_config"`
}

type CompressorPerRouteConfig struct {
	Type      string                      `yaml:"@type"`
	Overrides CompressorPerRouteOverrides `yaml:"overrides"`
}

// CompressorPerRoute builds the typed_per_filter_config entry that turns
// gzip response compression on for one route, overriding the
// listener-wide default (off).
func CompressorPerRoute() map[string]any {
	return map[string]any{
		"envoy.filters.http.compressor": CompressorPerRouteConfig{
			Type: TypeCompressorPerRoute,
			Overrides: CompressorPerRouteOverrides{
				ResponseDirectionConfig: RuntimeFeatureFlag{DefaultValue: true},
			},
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
