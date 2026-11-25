package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/component/resource"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	rules "github.com/metacubex/mihomo/rules/common"
	providers "github.com/metacubex/mihomo/rules/provider"
	"github.com/metacubex/mihomo/tunnel"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

const followAll = "fffff"

var serviceIDs = [...]string{"bilibili", "tencent-video", "iqiyi", "youku", "hunantv"}
var biliDomains = [][]string{
	{"bilibili.com", "biliapi.net", "biliapi.com", "b23.tv"},
	{"hdslb.com", "biliimg.com"},
	{"bilivideo.com", "bilivideo.cn", "bilivideo.net", "acgvideo.com"},
}

func suffix(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}
func biliCategory(host string) int {
	for i, domains := range biliDomains {
		for _, d := range domains {
			if suffix(host, d) {
				return i
			}
		}
	}
	return -1
}
func tencentVideo(m *C.Metadata) bool {
	host := m.RuleHost()
	for _, d := range []string{"v.qq.com", "video.qq.com", "qqvideo.gtimg.com", "v.smtcdns.com"} {
		if suffix(host, d) {
			return true
		}
	}
	switch host {
	case "v.streaming.qq.com", "ios.video.mpush.qq.com", "video-public-1258344701.shiply-cdn.qq.com", "video-search-1258344701.shiply-cdn.qq.com", "video-vip-1258344701.shiply-cdn.qq.com", "apd-pcdnvodstat.teg.tencent-cloud.net":
		return true
	}
	return false
}

type matcher func(*C.Metadata) bool
type route struct {
	rules.Base
	name, target string
	matches      matcher
}

func (r route) Match(m *C.Metadata, _ C.RuleMatchHelper) (bool, string) {
	return r.matches(m), r.target
}
func (r route) Payload() string      { return r.name }
func (r route) Adapter() string      { return r.target }
func (r route) RuleType() C.RuleType { return C.RuleSet }
func lan(m *C.Metadata) bool {
	ip := m.DstIP.Unmap()
	host := m.RuleHost()
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || suffix(host, "localhost") || suffix(host, "local")
}

type catalog struct {
	services             [len(serviceIDs)]matcher
	chinaDomain, chinaIP matcher
}

func loadSet(dir, id string, behavior P.RuleBehavior) (matcher, error) {
	// Read the immutable package once, with Mihomo's own MRS decoder.
	p := providers.NewRuleSetProvider(id, behavior, P.MrsRule, 0, resource.NewFileVehicle(filepath.Join(dir, id+".mrs")), nil, nil, nil)
	if err := p.Update(); err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	if p.Count() == 0 {
		return nil, fmt.Errorf("empty rule set %s", id)
	}
	return func(m *C.Metadata) bool { return p.Match(m, C.RuleMatchHelper{}) }, nil
}
func loadCatalog(dir string) (catalog, error) {
	var c catalog
	var err error
	for i, id := range serviceIDs {
		if id == "tencent-video" {
			c.services[i] = tencentVideo
			continue
		}
		c.services[i], err = loadSet(dir, id, P.Domain)
		if err != nil {
			return c, err
		}
	}
	bili := c.services[0]
	c.services[0] = func(m *C.Metadata) bool { return biliCategory(m.RuleHost()) >= 0 || bili(m) }
	c.chinaDomain, err = loadSet(dir, "cn-domain", P.Domain)
	if err != nil {
		return c, err
	}
	c.chinaIP, err = loadSet(dir, "cn-ip", P.IPCIDR)
	return c, err
}

type preferences struct {
	Mode    string `json:"mode"`
	Choices string `json:"choices"`
}

func preferenceFile(dir, name, fallback string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if os.IsNotExist(err) {
		return fallback, nil
	}
	return string(b), err
}
func readPreferences(dir string) (preferences, error) {
	var v preferences
	var err error
	v.Mode, err = preferenceFile(dir, "routing-mode", "smart")
	if err != nil {
		return v, err
	}
	v.Choices, err = preferenceFile(dir, "service-choices", followAll)
	if err != nil {
		return v, err
	}
	if (v.Mode != "smart" && v.Mode != "global") || len(v.Choices) != len(serviceIDs) || strings.Trim(v.Choices, "fdp") != "" {
		return v, fmt.Errorf("invalid routing preferences")
	}
	return v, nil
}
func (c catalog) rules(v preferences, custom ...C.Rule) []C.Rule {
	list := []C.Rule{&route{name: "LAN", target: "DIRECT", matches: lan}}
	if v.Mode == "smart" {
		// Explicit service choices precede defaults in case domain lists overlap.
		for _, explicit := range []bool{true, false} {
			if !explicit {
				list = append(list, custom...)
			}
			for i, id := range serviceIDs {
				choice := v.Choices[i]
				if (choice != 'f') != explicit {
					continue
				}
				target := "DIRECT"
				if choice == 'p' {
					target = "TV"
				}
				list = append(list, &route{name: id, target: target, matches: c.services[i]})
				if choice == 'p' {
					// Mihomo skips a TV rule if its node cannot carry UDP.
					list = append(list, &route{name: id, target: "REJECT", matches: c.services[i]})
				}
			}
		}
		list = append(list, &route{name: "CN domain", target: "DIRECT", matches: c.chinaDomain}, &route{name: "CN IP", target: "DIRECT", matches: c.chinaIP})
	}
	return append(list, rules.NewMatch("TV"), rules.NewMatch("REJECT"))
}

type policySession struct {
	management *control
	custom     atomic.Pointer[[]C.Rule]
	config     *config.Config
	catalog    catalog
	preferences
	stop, done chan struct{}
}

var policy *policySession

func newPolicy(cfg *config.Config) (*policySession, error) {
	v, err := readPreferences(home)
	if err != nil {
		return nil, err
	}
	c, err := loadCatalog("/pkg/assets/raw")
	if err != nil {
		return nil, err
	}
	p := &policySession{config: cfg, catalog: c, preferences: v, stop: make(chan struct{}), done: make(chan struct{})}
	p.management, err = newControl(p, home)
	if err != nil {
		return nil, err
	}
	cfg.Rules = c.rules(v, (*p.custom.Load())...)
	return p, nil
}

type routeReport struct {
	preferences
	Services [len(serviceIDs)]string `json:"services"`
	Bili     [3]string               `json:"bili"`
	Error    string                  `json:"error,omitempty"`
}

func (p *policySession) monitor() {
	defer close(p.done)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	report := routeReport{preferences: p.preferences}
	since := time.Now()
	var latest [len(serviceIDs) + 3]time.Time
	last := ""
	applied := p.custom.Load()
	for {
		v, err := readPreferences(home)
		if err != nil {
			report.Error = "无法读取分流设置"
		} else {
			report.Error = ""
			if next := p.custom.Load(); v != p.preferences || next != applied {
				applied = next
				p.preferences = v
				tunnel.UpdateRules(p.catalog.rules(v, (*applied)...), p.config.SubRules, p.config.RuleProviders)
				report = routeReport{preferences: v}
				latest = [len(serviceIDs) + 3]time.Time{}
				since = time.Now()
			}
		}
		statistic.DefaultManager.Range(func(conn statistic.Tracker) bool {
			t := conn.Info()
			if t.Start.Before(since) || t.DownloadTotal.Load() == 0 || t.Metadata == nil || len(t.Chain) == 0 {
				return true
			}
			egress := "proxy"
			if t.Chain[0] == "DIRECT" {
				egress = "direct"
			}
			for i, matches := range p.catalog.services {
				if matches(t.Metadata) && t.Start.After(latest[i]) {
					report.Services[i] = egress
					latest[i] = t.Start
				}
			}
			if i := biliCategory(t.Metadata.RuleHost()); i >= 0 && t.Start.After(latest[len(serviceIDs)+i]) {
				report.Bili[i] = egress
				latest[len(serviceIDs)+i] = t.Start
			}
			return true
		})
		b, _ := json.Marshal(report)
		if data := string(b); data != last {
			path := home + "/route-report"
			if err := os.WriteFile(path+".tmp", b, 0600); err == nil && os.Rename(path+".tmp", path) == nil {
				last = data
			}
		}
		select {
		case <-p.stop:
			return
		case <-ticker.C:
		}
	}
}
func stopPolicy() {
	if policy != nil {
		close(policy.stop)
		<-policy.done
		policy = nil
	}
}
