package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/metacubex/mihomo/common/yaml"
	"github.com/metacubex/mihomo/component/resource"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	rules "github.com/metacubex/mihomo/rules"
	rp "github.com/metacubex/mihomo/rules/provider"
	"github.com/metacubex/mihomo/tunnel"
)

type entry struct {
	ID, Kind, Value, Target, Format, Behavior string
	Enabled                                   bool
	Count                                     int
	Cache, Error                              string
	Updated, Attempt                          int64
}

var validHost = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func simple(e entry) (matcher, error) {
	switch e.Kind {
	case "cidr":
		prefix, err := netip.ParsePrefix(e.Value)
		if err != nil || !prefix.Addr().Is4() {
			return nil, errors.New("请输入 IPv4 网段，如 10.0.0.0/8")
		}
		return func(m *C.Metadata) bool { return prefix.Contains(m.DstIP.Unmap()) }, nil
	case "domain", "suffix":
		host := strings.ToLower(strings.TrimSuffix(e.Value, "."))
		if len(host) > 253 || !validHost.MatchString(host) {
			return nil, errors.New("域名格式不正确")
		}
		return func(m *C.Metadata) bool {
			h := strings.ToLower(strings.TrimSuffix(m.RuleHost(), "."))
			return h == host || e.Kind == "suffix" && suffix(h, host)
		}, nil
	}
	return nil, errors.New("未知规则类型")
}
func (c *control) loadRules() error {
	data, err := os.ReadFile(filepath.Join(c.dir, "rules.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if err = json.Unmarshal(data, &c.entries); err != nil {
			return err
		}
	}
	for i := range c.entries {
		e := &c.entries[i]
		var m matcher
		if e.Kind == "feed" {
			if e.Cache == "" {
				continue
			}
			m, _, err = compileFeed(*e, filepath.Join(c.dir, e.Cache))
		} else {
			m, err = simple(*e)
		}
		if err != nil {
			return err
		}
		c.compiled[e.ID] = m
	}
	c.installRules()
	return nil
}
func (c *control) installRules() {
	var list []C.Rule
	for _, e := range c.entries {
		m := c.compiled[e.ID]
		if !e.Enabled || m == nil {
			continue
		}
		list = append(list, &route{name: e.ID, target: e.Target, matches: m})
		if e.Target == "TV" {
			list = append(list, &route{name: e.ID, target: "REJECT", matches: m})
		}
	}
	c.session.custom.Store(&list)
}
func (c *control) commit(list []entry) error {
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	if err = c.privateWrite("rules.json", data); err != nil {
		return err
	}
	c.entries = list
	c.installRules()
	return nil
}
func (c *control) ruleCommand(q command) (any, error) {
	if q.Op == "rules" {
		return map[string]any{"entries": c.entries, "updating": c.updating}, nil
	}
	list := slices.Clone(c.entries)
	if q.Op == "add" {
		e := entry{Kind: q.Entry.Kind, Value: strings.TrimSpace(q.Entry.Value), Target: q.Entry.Target, Enabled: true, Format: q.Entry.Format, Behavior: q.Entry.Behavior}
		if e.Target != "TV" && e.Target != "DIRECT" {
			return nil, errors.New("未知出站")
		}
		id := make([]byte, 8)
		if _, err := rand.Read(id); err != nil {
			return nil, err
		}
		e.ID = hex.EncodeToString(id)
		if e.Kind == "feed" {
			u, err := url.Parse(e.Value)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" {
				return nil, errors.New("请输入 HTTPS 规则集链接")
			}
			if _, err = P.ParseBehavior(e.Behavior); err != nil {
				return nil, err
			}
			if _, err = P.ParseRuleFormat(e.Format); err != nil {
				return nil, err
			}
			if e.Format == "mrs" && e.Behavior == "classical" {
				return nil, errors.New("MRS 请选择域名或 IP 网段")
			}
		} else {
			m, err := simple(e)
			if err != nil {
				return nil, err
			}
			c.compiled[e.ID] = m
		}
		if err := c.commit(append(list, e)); err != nil {
			delete(c.compiled, e.ID)
			return nil, err
		}
		if e.Kind == "feed" {
			c.beginUpdate(e)
		}
		return nil, nil
	}
	i := slices.IndexFunc(list, func(e entry) bool { return e.ID == q.ID })
	if i < 0 {
		return nil, errors.New("规则不存在")
	}
	e := list[i]
	switch q.Op {
	case "toggle":
		list[i].Enabled = !e.Enabled
	case "target":
		if e.Target == "TV" {
			list[i].Target = "DIRECT"
		} else {
			list[i].Target = "TV"
		}
	case "move":
		j := i + q.Step
		if (q.Step != 1 && q.Step != -1) || j < 0 || j >= len(list) {
			return nil, errors.New("已到列表边界")
		}
		list[i], list[j] = list[j], list[i]
	case "delete":
		list = slices.Delete(list, i, i+1)
	case "update":
		if e.Kind != "feed" {
			return nil, errors.New("仅规则集可更新")
		}
		if len(c.updating) > 0 {
			return nil, errors.New("正在更新规则集")
		}
		c.beginUpdate(e)
		return nil, nil
	default:
		return nil, errors.New("操作无效")
	}
	if err := c.commit(list); err != nil {
		return nil, err
	}
	if q.Op == "delete" {
		delete(c.compiled, e.ID)
		if e.Cache != "" {
			os.Remove(filepath.Join(c.dir, e.Cache))
		}
	}
	return nil, nil
}
func (c *control) refreshDue() {
	if len(c.updating) > 0 {
		return
	}
	for _, e := range c.entries {
		if e.Enabled && e.Kind == "feed" && time.Now().Unix()-e.Attempt >= 86400 {
			c.beginUpdate(e)
			return
		}
	}
}
func (c *control) beginUpdate(e entry) {
	if len(c.updating) > 0 {
		return
	}
	c.updating[e.ID] = true
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		filename, m, count, err := c.fetch(e)
		c.finishUpdate(e, filename, m, count, err)
	}()
}
func (c *control) finishUpdate(e entry, filename string, m matcher, count int, fetchErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.updating, e.ID)
	list := slices.Clone(c.entries)
	i := slices.IndexFunc(list, func(it entry) bool { return it.ID == e.ID })
	if i < 0 || c.ctx.Err() != nil {
		if filename != "" && filename != e.Cache {
			os.Remove(filepath.Join(c.dir, filename))
		}
		return
	}
	item := &list[i]
	item.Attempt = time.Now().Unix()
	old := c.compiled[e.ID]
	if fetchErr != nil {
		item.Error = fetchErr.Error()
	} else {
		item.Cache = filename
		item.Count = count
		item.Updated = item.Attempt
		item.Error = ""
		c.compiled[e.ID] = m
	}
	if err := c.commit(list); err != nil {
		c.compiled[e.ID] = old
		if filename != "" && filename != e.Cache {
			os.Remove(filepath.Join(c.dir, filename))
		}
		c.entries[i].Error = "无法保存更新"
		return
	}
	if e.Cache != "" && item.Cache != e.Cache {
		os.Remove(filepath.Join(c.dir, e.Cache))
	}
}

const maxFeed = 4 << 20

func (c *control) fetch(e entry) (string, matcher, int, error) {
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	tr := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		m := &C.Metadata{NetWork: C.TCP}
		if err := m.SetRemoteAddress(address); err != nil {
			return nil, err
		}
		return tunnel.Proxies()["TV"].DialContext(ctx, m)
	}}
	defer tr.CloseIdleConnections()
	client := http.Client{Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || req.URL.Scheme != "https" {
			return errors.New("重定向无效")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, "GET", e.Value, nil)
	if err != nil {
		return "", nil, 0, errors.New("规则集地址无效")
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, 0, errors.New("下载失败，请检查节点和链接")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", nil, 0, fmt.Errorf("下载返回 HTTP %d", resp.StatusCode)
	}
	f, err := os.CreateTemp(c.dir, "ruleset-*.tmp")
	if err != nil {
		return "", nil, 0, errors.New("无法创建缓存")
	}
	path := f.Name()
	defer os.Remove(path)
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxFeed+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil || n == 0 || n > maxFeed {
		return "", nil, 0, errors.New("规则集下载不完整或超过 4 MiB")
	}
	m, count, err := compileFeed(e, path)
	if err != nil {
		return "", nil, 0, fmt.Errorf("解析失败：%s", err)
	}
	name := "feed-" + e.ID + "-" + hex.EncodeToString(h.Sum(nil))[:16]
	if err = os.Rename(path, filepath.Join(c.dir, name)); err != nil {
		return "", nil, 0, errors.New("缓存保存失败")
	}
	return name, m, count, nil
}
func compileFeed(e entry, path string) (matcher, int, error) {
	behavior, err := P.ParseBehavior(e.Behavior)
	if err != nil {
		return nil, 0, err
	}
	format, err := P.ParseRuleFormat(e.Format)
	if err != nil {
		return nil, 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, errors.New("缓存不可读")
	}
	var expected int
	var provider P.RuleProvider
	if format == P.MrsRule {
		expected, err = validateMRS(data, behavior)
		if err != nil {
			return nil, 0, err
		}
		provider = rp.NewRuleSetProvider(e.ID, behavior, format, 0, resource.NewFileVehicle(path), nil, nil, nil)
		if err = provider.Update(); err != nil {
			return nil, 0, errors.New("MRS 内容无效")
		}
	} else {
		var lines []string
		if format == P.YamlRule {
			var doc struct {
				Payload []string `yaml:"payload"`
			}
			if err = yaml.Unmarshal(data, &doc); err != nil {
				return nil, 0, errors.New("YAML 格式无效")
			}
			lines = doc.Payload
		} else {
			scan := bufio.NewScanner(bytes.NewReader(data))
			for scan.Scan() {
				line := strings.TrimSpace(scan.Text())
				if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "//") {
					lines = append(lines, line)
				}
			}
			if scan.Err() != nil {
				return nil, 0, errors.New("规则行过长")
			}
		}
		expected = len(lines)
		if expected == 0 || expected > 100000 {
			return nil, 0, errors.New("规则数量无效")
		}
		provider = rp.NewInlineProvider(e.ID, behavior, lines, rules.ParseRule)
	}
	if provider.Count() == 0 || provider.Count() != expected {
		return nil, 0, errors.New("包含无效规则")
	}
	return func(m *C.Metadata) bool { return provider.Match(m, C.RuleMatchHelper{}) }, provider.Count(), nil
}
