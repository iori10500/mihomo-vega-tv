package main

import (
	C "github.com/metacubex/mihomo/constant"
	rp "github.com/metacubex/mihomo/rules/provider"
	"github.com/metacubex/mihomo/tunnel"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func TestRuleSubscriptions(t *testing.T) {
	rp.SetTunnel(tunnel.Tunnel)
	for _, x := range []struct {
		format, behavior, input, host, ip string
		ok                                bool
	}{
		{"mrs", "domain", "assets/raw/bilibili.mrs", "api.bilibili.com", "1.1.1.1", true}, {"mrs", "ipcidr", "assets/raw/cn-ip.mrs", "", "223.5.5.5", true},
		{"yaml", "domain", "payload:\n  - '+.example.org'\n", "a.example.org", "1.1.1.1", true},
		{"text", "ipcidr", "203.0.113.0/24\n", "", "203.0.113.2", true}, {"yaml", "classical", "payload:\n  - DOMAIN-SUFFIX,example.org\n", "a.example.org", "1.1.1.1", true},
		{"text", "ipcidr", "203.0.113.0/24\nwrong\n", "", "1.1.1.1", false}, {"mrs", "ipcidr", "assets/raw/bilibili.mrs", "", "1.1.1.1", false},
	} {
		path := x.input
		if x.format != "mrs" {
			path = filepath.Join(t.TempDir(), "feed")
			if err := os.WriteFile(path, []byte(x.input), 0600); err != nil {
				t.Fatal(err)
			}
		}
		m, n, err := compileFeed(entry{ID: "fixture", Format: x.format, Behavior: x.behavior}, path)
		if !x.ok {
			if err == nil {
				t.Error("invalid input accepted", x.behavior)
			}
			continue
		}
		if err != nil || n < 1 {
			t.Fatal(x.format, x.behavior, err)
		}
		if !m(&C.Metadata{Host: x.host, DstIP: netip.MustParseAddr(x.ip)}) {
			t.Error("missing match", x.behavior)
		}
	}
}
func TestCustomPrecedence(t *testing.T) {
	rp.SetTunnel(tunnel.Tunnel)
	c, err := loadCatalog("assets/raw")
	if err != nil {
		t.Fatal(err)
	}
	const host = "api.bilibili.com"
	m, _ := simple(entry{Kind: "domain", Value: host})
	custom := &route{name: "user", target: "TV", matches: m}
	for _, x := range []struct {
		prefs      preferences
		ip, target string
	}{{preferences{"smart", followAll}, "223.5.5.5", "TV"}, {preferences{"smart", "dffff"}, "223.5.5.5", "DIRECT"}, {preferences{"global", followAll}, "192.168.0.1", "DIRECT"}} {
		for _, r := range c.rules(x.prefs, custom) {
			if hit, to := r.Match(&C.Metadata{Host: host, DstIP: netip.MustParseAddr(x.ip)}, C.RuleMatchHelper{}); hit {
				if to != x.target {
					t.Error(to, x.target)
				}
				break
			}
		}
	}
}

func TestRuleSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	open := func() *control {
		t.Helper()
		c, err := newControl(&policySession{}, dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(c.close)
		return c
	}
	c := open()
	run := func(q command) {
		t.Helper()
		if _, err := c.ruleCommand(q); err != nil {
			t.Fatal(err)
		}
	}
	run(command{Op: "add", Entry: entry{Kind: "domain", Value: "example.org", Target: "TV"}})
	run(command{Op: "add", Entry: entry{Kind: "cidr", Value: "203.0.113.0/24", Target: "DIRECT"}})
	domain, ip := c.entries[0].ID, c.entries[1].ID
	run(command{Op: "move", ID: ip, Step: -1})
	run(command{Op: "target", ID: ip})
	run(command{Op: "toggle", ID: domain})
	c = open()
	if len(c.entries) != 2 || c.entries[0].ID != ip || c.entries[0].Target != "TV" || c.entries[1].Enabled || len(*c.session.custom.Load()) != 2 {
		t.Fatal("settings or matchers lost after restart")
	}
	run(command{Op: "delete", ID: ip})
	c = open()
	if len(c.entries) != 1 || c.entries[0].ID != domain || len(*c.session.custom.Load()) != 0 {
		t.Fatal("delete not persisted")
	}
}

func TestFailedFeedUpdateKeepsLastGoodVersion(t *testing.T) {
	dir := t.TempDir()
	c, err := newControl(&policySession{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	e := entry{ID: "fixture", Kind: "feed", Enabled: true, Format: "text", Behavior: "domain", Target: "DIRECT"}
	if err = c.commit([]entry{e}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "feed-fixture-demo")
	if err = os.WriteFile(path, []byte("+.example.org\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, count, err := compileFeed(e, path)
	if err != nil {
		t.Fatal(err)
	}
	c.finishUpdate(e, filepath.Base(path), m, count, nil)
	good := c.entries[0]
	c.finishUpdate(good, "", nil, 0, os.ErrInvalid)
	restored, err := newControl(&policySession{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.close()
	got := restored.entries[0]
	if got.Error == "" || got.Cache != good.Cache || got.Count != 1 || got.Updated != good.Updated || !restored.compiled[e.ID](&C.Metadata{Host: "test.example.org"}) {
		t.Fatal("last good feed lost")
	}
}
