package main

import "C"
import (
	"net"

	"net/netip"
	"os"
	"runtime/debug"

	"github.com/metacubex/mihomo/config"
	constant "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/listener"
	"github.com/metacubex/mihomo/tunnel"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

const home = "/data/iori-mihomo"

func configureGroups(raw *config.RawConfig) {
	// Upgrade both saved and newly imported profiles without rewriting secrets on disk.
	raw.ProxyGroup = []map[string]any{
		{"name": autoGroup, "type": "url-test", "use": []string{"nodes"}, "url": latencyURL, "interval": 300},
		{"name": "TV", "type": "select", "proxies": []string{autoGroup}, "use": []string{"nodes"}},
	}
}

//export TVCoreStart
func TVCoreStart(host *C.char) C.int {
	debug.SetMemoryLimit(24 << 20)
	constant.SetHomeDir(home)
	configPath := home + "/config.json"
	if _, err := os.Stat(home + "/pending.json"); err == nil {
		configPath = home + "/pending.json"
	} else if !os.IsNotExist(err) {
		return 1
	}
	constant.SetConfig(configPath)
	if _, err := os.Stat(constant.Path.Config()); err != nil {
		return 1
	}
	if err := config.Init(home); err != nil {
		return 2
	}
	ip, err := netip.ParseAddr(C.GoString(host))
	if err != nil || !ip.Is4() {
		return 4
	}
	linkName := ""
	interfaces, err := net.Interfaces()
	if err != nil {
		return 4
	}
	for _, link := range interfaces {
		addresses, err := link.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addresses {
			prefix, err := netip.ParsePrefix(a.String())
			if err == nil && prefix.Addr().Unmap() == ip {
				linkName = link.Name
			}
		}
	}
	if linkName == "" {
		return 4
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return 1
	}
	raw, err := config.UnmarshalRawConfig(data)
	if err != nil {
		return 3
	}
	// The Vega TUN passes destination IPs. Match the indicated hostname while
	// keeping the original CDN IP selected by the application.
	raw.Sniffer = config.RawSniffer{Enable: true, ParsePureIp: true, ForceDnsMapping: true,
		Sniff: map[string]config.RawSniffingConfig{
			"HTTP": {Ports: []string{"80", "8080"}},
			"TLS":  {Ports: []string{"443"}},
			"QUIC": {Ports: []string{"443"}},
		},
	}
	configureGroups(raw)
	cfg, err := config.ParseRawConfig(raw)
	if err != nil {
		return 3
	}
	cfg.General.Interface = linkName
	cfg.General.AllowLan = false
	cfg.General.MixedPort = 0
	cfg.General.Mode = tunnel.Rule
	session, err := newPolicy(cfg)
	if err != nil {
		return 7
	}
	hub.ApplyConfig(cfg)
	policy = session
	go session.monitor()
	if nodes := tunnel.Providers()["nodes"]; nodes == nil || nodes.Count() == 0 {
		TVCoreStop()
		return 6
	}
	if err := session.startControl(); err != nil {
		TVCoreStop()
		return 8
	}
	return 0
}

//export TVCoreStop
func TVCoreStop() {
	if policy != nil && policy.management != nil {
		policy.management.close()
	}
	stopPolicy()
	listener.ReCreateMixed(0, tunnel.Tunnel)
	tunnel.SetMode(tunnel.Direct)
	statistic.DefaultManager.Range(func(connection statistic.Tracker) bool {
		_ = connection.Close()
		return true
	})
}
func main() {}
