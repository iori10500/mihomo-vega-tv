package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/outboundgroup"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"
)

const autoGroup = "TV · Auto"
const latencyURL = "https://www.gstatic.com/generate_204"

type command struct {
	Op    string
	Name  string
	ID    string
	Step  int
	Entry entry
}
type result struct {
	Delay int
	Mbps  float64
	At    int64
	Error string
}
type nodeInfo struct {
	Name              string
	Latency, Download result
}
type control struct {
	dir                   string
	mu                    sync.Mutex
	ctx                   context.Context
	cancel                context.CancelFunc
	wg                    sync.WaitGroup
	ln                    net.Listener
	session               *policySession
	selected, notice, job string
	cancelTest            context.CancelFunc
	latency, speed        map[string]result
	entries               []entry
	compiled              map[string]matcher
	updating              map[string]bool
}

func (c *control) privateWrite(name string, data []byte) error {
	path := filepath.Join(c.dir, name)
	if err := os.WriteFile(path+".new", data, 0600); err != nil {
		return err
	}
	return os.Rename(path+".new", path)
}
func newControl(s *policySession, dir string) (*control, error) {
	c := &control{dir: dir, session: s, latency: map[string]result{}, speed: map[string]result{}, compiled: map[string]matcher{}, updating: map[string]bool{}}
	var err error
	if c.selected, err = preferenceFile(dir, "selected-node", ""); err != nil {
		return nil, err
	}
	if err = c.loadRules(); err != nil {
		return nil, err
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	return c, nil
}
// Runtime sockets must stay out of /data. Vega syncs that tree during package updates.
const socketDir = "/cache/iori-mihomo"

func (p *policySession) startControl() error {
	c := p.management
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(socketDir, "management.sock")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	c.ln = ln
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	c.restoreSelection()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			var req command
			var reply []byte
			if err = json.NewDecoder(io.LimitReader(conn, 16384)).Decode(&req); err == nil {
				c.mu.Lock()
				var value any
				value, err = c.handle(req)
				if err == nil {
					reply, err = json.Marshal(value)
				}
				c.mu.Unlock()
			}
			if err != nil {
				reply, _ = json.Marshal(map[string]string{"error": err.Error()})
			}
			conn.Write(append(reply, '\n'))
			conn.Close()
		}
	}()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			c.mu.Lock()
			c.restoreSelection()
			c.refreshDue()
			c.mu.Unlock()
			select {
			case <-c.ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
	return nil
}
func (c *control) close() {
	c.cancel()
	if c.ln != nil {
		c.ln.Close()
	}
	c.wg.Wait()
}
func nodes() []C.Proxy { return tunnel.Providers()["nodes"].Proxies() }
func findNode(name string) C.Proxy {
	for _, n := range nodes() {
		if n.Name() == name {
			return n
		}
	}
	return nil
}
func selector() *outboundgroup.Selector {
	return tunnel.Proxies()["TV"].Adapter().(*outboundgroup.Selector)
}
func (c *control) restoreSelection() {
	name := c.selected
	if name != "" && findNode(name) == nil {
		if err := c.privateWrite("selected-node", nil); err != nil {
			c.notice = "节点偏好保存失败"
			return
		}
		c.selected = ""
		name = ""
		c.notice = "原节点已从订阅移除，已恢复自动选择"
	}
	if name == "" {
		name = autoGroup
	}
	if selector().Now() != name {
		if err := selector().Set(name); err != nil {
			c.notice = "节点组暂不可用"
		}
	}
}
func (c *control) handle(q command) (any, error) {
	switch q.Op {
	case "nodes":
		c.restoreSelection()
		list := make([]nodeInfo, 0, len(nodes()))
		for _, n := range nodes() {
			item := nodeInfo{Name: n.Name(), Latency: c.latency[n.Name()], Download: c.speed[n.Name()]}
			if item.Latency.At == 0 {
				hist := n.DelayHistory()
				if len(hist) > 0 {
					h := hist[len(hist)-1]
					item.Latency = result{Delay: int(h.Delay), At: h.Time.Unix()}
					if h.Delay == 0 {
						item.Latency.Error = "超时"
					}
				}
			}
			list = append(list, item)
		}
		current := selector().Now()
		if current == autoGroup {
			current = tunnel.Proxies()[autoGroup].Adapter().(*outboundgroup.URLTest).Now()
		}
		return map[string]any{"nodes": list, "selected": c.selected, "current": current, "notice": c.notice, "job": c.job}, nil
	case "select":
		if q.Name != "" && (q.Name == autoGroup || findNode(q.Name) == nil) {
			return nil, errors.New("节点不可用")
		}
		if err := c.privateWrite("selected-node", []byte(q.Name)); err != nil {
			return nil, err
		}
		c.selected = q.Name
		c.notice = ""
		c.restoreSelection()
	case "latency", "download", "all":
		return nil, c.startTest(q)
	case "cancel":
		if c.cancelTest != nil {
			c.cancelTest()
		}
	default:
		return c.ruleCommand(q)
	}
	return nil, nil
}
func (c *control) startTest(q command) error {
	if c.cancelTest != nil {
		return errors.New("测速正在进行")
	}
	targets := nodes()
	if q.Op != "all" {
		n := findNode(q.Name)
		if n == nil {
			return errors.New("节点不可用")
		}
		targets = []C.Proxy{n}
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.cancelTest = cancel
	c.job = q.Op
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		jobs := make(chan C.Proxy)
		var workers sync.WaitGroup
		count := 1
		if q.Op == "all" {
			count = 2
		}
		for i := 0; i < count; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for n := range jobs {
					if ctx.Err() != nil {
						return
					}
					r := probe(ctx, n, q.Op == "download")
					c.mu.Lock()
					if ctx.Err() == nil {
						if q.Op == "download" {
							c.speed[n.Name()] = r
						} else {
							c.latency[n.Name()] = r
						}
					}
					c.mu.Unlock()
				}
			}()
		}
	send:
		for _, n := range targets {
			select {
			case jobs <- n:
			case <-ctx.Done():
				break send
			}
		}
		close(jobs)
		workers.Wait()
		c.mu.Lock()
		c.job = ""
		c.cancelTest = nil
		c.mu.Unlock()
		cancel()
	}()
	return nil
}
func probe(parent context.Context, n C.Proxy, download bool) result {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	r := result{At: time.Now().Unix()}
	if !download {
		d, err := n.URLTest(ctx, latencyURL, nil)
		if err != nil {
			r.Error = "超时或不可达"
		} else {
			r.Delay = int(d)
		}
		return r
	}
	const size = 4 << 20
	tr := &http.Transport{DisableCompression: true, DisableKeepAlives: true, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		m := &C.Metadata{NetWork: C.TCP}
		if err := m.SetRemoteAddress(address); err != nil {
			return nil, err
		}
		return n.DialContext(ctx, m)
	}}
	defer tr.CloseIdleConnections()
	client := http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://speed.cloudflare.com/__down?bytes=%d", size), nil)
	res, err := client.Do(req)
	if err != nil {
		r.Error = "测速端点不可达"
		return r
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		r.Error = "测速端点响应异常"
		return r
	}
	start := time.Now()
	amount, err := io.Copy(io.Discard, io.LimitReader(res.Body, size))
	if err != nil || amount != size {
		r.Error = "下载未完成"
		return r
	}
	r.Mbps = float64(amount) * 8 / time.Since(start).Seconds() / 1e6
	return r
}
