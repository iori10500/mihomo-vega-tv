package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"
)

// Decode the same profile shape shipped by 0.3.0; inline nodes keep the test offline.
func TestNodeSelectorMigration(t *testing.T) {
	dir := t.TempDir()
	C.SetHomeDir(dir)
	raw, err := config.UnmarshalRawConfig([]byte(`{"proxy-providers":{"nodes":{"type":"inline","payload":[{"name":"node-a","type":"direct"},{"name":"node-b","type":"direct"}]}},"proxy-groups":[{"name":"TV","type":"url-test","use":["nodes"]}],"rules":["MATCH,TV"]}`))
	if err != nil {
		t.Fatal(err)
	}
	configureGroups(raw)
	cfg, err := config.ParseRawConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	tunnel.UpdateProxies(cfg.Proxies, cfg.Providers)
	c, err := newControl(&policySession{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	c.restoreSelection()
	if selector().Now() != autoGroup {
		t.Fatal("default is not auto")
	}
	if _, err = c.handle(command{Op: "select", Name: "node-b"}); err != nil {
		t.Fatal(err)
	}
	if selector().Now() != "node-b" {
		t.Fatalf("selection not applied")
	}
	restored, err := newControl(&policySession{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.close()
	if restored.selected != "node-b" {
		t.Fatal("selection lost")
	}
	if _, err = c.handle(command{Op: "select", Name: "missing"}); err == nil {
		t.Fatal("invalid node accepted")
	}
	c.selected = "removed"
	c.restoreSelection()
	if c.selected != "" || selector().Now() != autoGroup || c.notice == "" {
		t.Fatal("missing node fallback")
	}
	// Probe a different candidate directly and leave the foreground selector alone.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer srv.Close()
	n := findNode("node-a")
	if _, err = n.URLTest(context.Background(), srv.URL, nil); err != nil {
		t.Fatal(err)
	}
	if selector().Now() != autoGroup {
		t.Fatal("test altered selection")
	}
	if _, ok := cfg.Proxies[autoGroup].Adapter().(*outboundgroup.URLTest); !ok {
		t.Fatal("missing automatic group")
	}
}
