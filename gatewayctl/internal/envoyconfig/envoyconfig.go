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
	Cluster       string `yaml:"cluster"`
	PrefixRewrite string `yaml:"prefix_rewrite,omitempty"`
	Timeout       string `yaml:"timeout"`
}

type Route struct {
	Match RouteMatch  `yaml:"match"`
	Route RouteAction `yaml:"route"`
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

type HTTPFilter struct {
	Name        string            `yaml:"name"`
	TypedConfig RouterTypedConfig `yaml:"typed_config"`
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
