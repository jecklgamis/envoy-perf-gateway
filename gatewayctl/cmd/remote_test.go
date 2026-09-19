package cmd

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
	ec "github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/envoyconfig"
	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/render"
)

// renderThenParseLDS exercises the exact path a remote deployment's config
// takes: values.yaml -> render.BuildLDS -> yaml.Marshal (what gets pushed)
// -> yaml.Unmarshal (what fetchRemoteFile/generate-values sees come back).
// This is the same round-trip generate-values relies on for fidelity,
// without needing a real config server.
func renderThenParseLDS(t *testing.T, v config.Values) ec.LDS {
	t.Helper()
	body, err := yaml.Marshal(render.BuildLDS(v))
	if err != nil {
		t.Fatalf("marshaling LDS: %v", err)
	}
	var lds ec.LDS
	if err := yaml.Unmarshal(body, &lds); err != nil {
		t.Fatalf("unmarshaling LDS: %v\n%s", err, body)
	}
	return lds
}

func TestFindRemoteRouteCatchAllPrefix(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "httpbin", Host: "httpbin.org", Port: 443, TLS: true, RoutePrefix: "/httpbin/", Timeout: "15s"},
	}}
	lds := renderThenParseLDS(t, v)

	info, ok := findRemoteRoute(lds, "httpbin")
	if !ok {
		t.Fatal("expected a route for httpbin, found none")
	}
	if info.Domain != "" || info.RoutePrefix != "/httpbin/" || info.HostRewrite != "" {
		t.Errorf("got %+v, want {Domain:\"\" RoutePrefix:/httpbin/ HostRewrite:\"\"}", info)
	}
}

func TestFindRemoteRouteDomainOnly(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "svc-a", Host: "svc-a.internal", Port: 8080, Domain: "frontend-a.test.local", Timeout: "15s"},
	}}
	lds := renderThenParseLDS(t, v)

	info, ok := findRemoteRoute(lds, "svc-a")
	if !ok {
		t.Fatal("expected a route for svc-a, found none")
	}
	if info.Domain != "frontend-a.test.local" || info.RoutePrefix != "" {
		t.Errorf("got %+v, want {Domain:frontend-a.test.local RoutePrefix:\"\"}", info)
	}
}

func TestFindRemoteRouteDomainPlusPrefix(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "svc-b", Host: "svc-b.internal", Port: 8080, Domain: "frontend-b.test.local", RoutePrefix: "/api/", Timeout: "15s"},
	}}
	lds := renderThenParseLDS(t, v)

	info, ok := findRemoteRoute(lds, "svc-b")
	if !ok {
		t.Fatal("expected a route for svc-b, found none")
	}
	if info.Domain != "frontend-b.test.local" || info.RoutePrefix != "/api/" {
		t.Errorf("got %+v, want {Domain:frontend-b.test.local RoutePrefix:/api/}", info)
	}
}

func TestFindRemoteRouteHostRewrite(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "serverprobes", Host: "www.serverprobes.com", Port: 443, TLS: true,
			RoutePrefix: "/serverprobes/", HostRewrite: "www.serverprobes.com", Timeout: "15s"},
	}}
	lds := renderThenParseLDS(t, v)

	info, ok := findRemoteRoute(lds, "serverprobes")
	if !ok {
		t.Fatal("expected a route for serverprobes, found none")
	}
	if info.HostRewrite != "www.serverprobes.com" {
		t.Errorf("got HostRewrite=%q, want www.serverprobes.com", info.HostRewrite)
	}
}

func TestFindRemoteRouteNoRoute(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "no-route", Host: "internal.only", Port: 9000, Timeout: "15s"},
	}}
	lds := renderThenParseLDS(t, v)

	if _, ok := findRemoteRoute(lds, "no-route"); ok {
		t.Error("expected no route for a backend with neither --domain nor --route-prefix")
	}
}

func TestFindRemoteRouteDoesNotConfuseDifferentBackends(t *testing.T) {
	v := config.Values{Backends: []config.Backend{
		{Name: "a", Host: "a.internal", Port: 80, RoutePrefix: "/a/", Timeout: "15s"},
		{Name: "b", Host: "b.internal", Port: 80, Domain: "b.test.local", Timeout: "15s"},
	}}
	lds := renderThenParseLDS(t, v)

	infoA, ok := findRemoteRoute(lds, "a")
	if !ok || infoA.RoutePrefix != "/a/" || infoA.Domain != "" {
		t.Errorf("route a: got %+v, ok=%v", infoA, ok)
	}
	infoB, ok := findRemoteRoute(lds, "b")
	if !ok || infoB.Domain != "b.test.local" || infoB.RoutePrefix != "" {
		t.Errorf("route b: got %+v, ok=%v", infoB, ok)
	}
}
