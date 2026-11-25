package main

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	providers "github.com/metacubex/mihomo/rules/provider"
	"github.com/metacubex/mihomo/tunnel"
)

func TestRouting(t *testing.T) {
	providers.SetTunnel(tunnel.Tunnel)
	c, err := loadCatalog("assets/raw")
	if err != nil {
		t.Fatal(err)
	}
	check := func(v preferences, host, ip, target string) {
		t.Helper()
		m := &C.Metadata{Host: host, DstIP: netip.MustParseAddr(ip)}
		for _, r := range c.rules(v) {
			if match, got := r.Match(m, C.RuleMatchHelper{}); match {
				if got != target {
					t.Errorf("%v %s %s matched %s => %s, want %s", v, host, ip, r.Payload(), got, target)
				}
				return
			}
		}
		t.Fatal("missing final rule")
	}
	hosts := [][]string{{"api.bilibili.com", "i0.hdslb.com", "xy.bilivideo.com"}, {"v.qq.com", "x.qqvideo.gtimg.com"}, {"iqiyi.com", "video.qiyi.com"}, {"youku.com", "vali.youku.com"}, {"www.mgtv.com", "hunantv.com"}}
	for i, domains := range hosts {
		for _, choice := range []byte("fdp") {
			choices := []byte(followAll)
			choices[i] = choice
			v := preferences{"smart", string(choices)}
			for _, host := range domains {
				want := "DIRECT"
				if choice == 'p' {
					want = "TV"
				}
				check(v, host, "1.1.1.1", want)
				check(v, host, "223.5.5.5", want)
				check(v, host, "192.168.1.1", "DIRECT")
				check(preferences{"global", string(choices)}, host, "223.5.5.5", "TV")
			}
		}
	}
	v := preferences{"smart", followAll}
	for _, x := range [][3]string{{"www.baidu.com", "1.1.1.1", "DIRECT"}, {"", "223.5.5.5", "DIRECT"}, {"", "1.1.1.1", "TV"}, {"example.org", "1.1.1.1", "TV"}, {"", "192.168.1.8", "DIRECT"}, {"", "127.0.0.1", "DIRECT"}, {"printer.local", "1.1.1.1", "DIRECT"}, {"", "fd00::4", "DIRECT"}} {
		check(v, x[0], x[1], x[2])
	}
	check(preferences{"smart", "fpfff"}, "mail.qq.com", "223.5.5.5", "DIRECT")
	check(preferences{"smart", "ppppp"}, "bilibili.com.example.org", "1.1.1.1", "TV")
	for _, m := range c.services {
		if m(&C.Metadata{Host: "bilibili.com.example.org"}) {
			t.Fatal("substring match")
		}
	}
	check(preferences{"global", "ppppp"}, "v.qq.com", "192.168.1.8", "DIRECT")
	// The upstream tunnel skips a proxy rule when the selected node lacks UDP.
	for _, prefs := range []preferences{{"smart", "pffff"}, {"global", followAll}} {
		m := &C.Metadata{Host: "api.bilibili.com", DstIP: netip.MustParseAddr("223.5.5.5")}
		next := ""
		for _, r := range c.rules(prefs) {
			if hit, dest := r.Match(m, C.RuleMatchHelper{}); hit && dest != "TV" {
				next = dest
				break
			}
		}
		if next != "REJECT" {
			t.Errorf("%v UDP fallback = %s", prefs, next)
		}
	}
}
func TestSavedPreferences(t *testing.T) {
	dir := t.TempDir()
	read := func(want preferences) {
		t.Helper()
		got, err := readPreferences(dir)
		if err != nil || got != want {
			t.Fatalf("%v %v", got, err)
		}
	}
	write := func(file, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, file), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	read(preferences{"smart", followAll})
	write("routing-mode", "global")
	read(preferences{"global", followAll})
	write("service-choices", "pdfdf")
	read(preferences{"global", "pdfdf"})
	write("routing-mode", "smart")
	read(preferences{"smart", "pdfdf"})
	write("service-choices", "ffffx")
	if _, err := readPreferences(dir); err == nil {
		t.Fatal("invalid preference accepted")
	}
}
