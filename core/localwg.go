package main

/*
#include <stdint.h>
*/
import "C"
import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"

	"time"
	"unsafe"

	CX "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/listener/sing"
	singtun "github.com/metacubex/mihomo/listener/sing_tun"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel"
	ipstack "github.com/metacubex/sing-tun"
	"github.com/metacubex/wireguard-go/conn"
	"github.com/metacubex/wireguard-go/device"
	wgtun "github.com/metacubex/wireguard-go/tun"
)

// Each channel preserves one IP packet between WireGuard and the userspace TCP/IP stack.
type packetPipe struct {
	incoming, outgoing chan []byte

	done chan struct{}
	once sync.Once
}

func (p *packetPipe) receive(from chan []byte, into []byte) (int, error) {
	select {
	case packet := <-from:
		if len(packet) > len(into) {
			return 0, io.ErrShortBuffer
		}
		return copy(into, packet), nil
	case <-p.done:
		return 0, os.ErrClosed
	}
}
func (p *packetPipe) send(to chan []byte, data []byte) (int, error) {
	packet := append([]byte(nil), data...)
	select {
	case to <- packet:
		return len(data), nil
	case <-p.done:
		return 0, os.ErrClosed
	}
}
func (p *packetPipe) Read(b []byte) (int, error)  { return p.receive(p.incoming, b) }
func (p *packetPipe) Write(b []byte) (int, error) { return p.send(p.outgoing, b) }
func (p *packetPipe) Close() error                { p.once.Do(func() { close(p.done) }); return nil }

type wireDevice struct {
	pipe   *packetPipe
	events chan wgtun.Event
	once   sync.Once
}

func (w *wireDevice) File() *os.File             { return nil }
func (w *wireDevice) Name() (string, error)      { return "mihomo-local", nil }
func (w *wireDevice) MTU() (int, error)          { return 1280, nil }
func (w *wireDevice) BatchSize() int             { return 1 }
func (w *wireDevice) Events() <-chan wgtun.Event { return w.events }
func (w *wireDevice) Close() error               { w.once.Do(func() { w.pipe.Close(); close(w.events) }); return nil }
func (w *wireDevice) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	n, err := w.pipe.receive(w.pipe.outgoing, bufs[0][offset:])
	if err != nil {
		return 0, err
	}
	sizes[0] = n
	return 1, nil
}
func (w *wireDevice) Write(bufs [][]byte, offset int) (int, error) {
	for i, b := range bufs {
		if _, err := w.pipe.send(w.pipe.incoming, b[offset:]); err != nil {
			return i, err
		}
	}
	return len(bufs), nil
}

var localWG *device.Device
var localStack ipstack.Stack
var healthListener net.Listener

//export TVLocalWGStart
func TVLocalWGStart(clientPrivate, clientPublic, serverPublic *C.uint8_t) C.int {
	TVLocalWGStop()
	client, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return 1
	}
	server, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return 1
	}
	h, err := sing.NewListenerHandler(sing.ListenerConfig{Tunnel: tunnel.Tunnel, Type: CX.TUN})
	if err != nil {
		return 2
	}
	v4 := []netip.Prefix{netip.MustParsePrefix("10.203.77.1/30")}
	v6 := []netip.Prefix{netip.MustParsePrefix("fd6a:1142:79::1/126")}
	handler := &singtun.ListenerHandler{ListenerHandler: h, Inet4Address: v4, Inet6Address: v6, DisableICMPForwarding: true}
	p := &packetPipe{incoming: make(chan []byte, 32), outgoing: make(chan []byte, 32), done: make(chan struct{})}
	st, err := ipstack.NewMipstack(ipstack.StackOptions{Context: context.Background(), Tun: p, TunOptions: ipstack.Options{MTU: 1280, Inet4Address: v4, Inet6Address: v6}, UDPTimeout: 2 * time.Minute, ICMPTimeout: 10 * time.Second, Handler: handler, Logger: log.SingLogger})
	if err != nil {
		return 3
	}
	if err = st.Start(); err != nil {
		p.Close()
		return 3
	}
	w := &wireDevice{pipe: p, events: make(chan wgtun.Event, 1)}
	wg := device.NewDevice(w, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""), 1)
	// The only peer is Vega's VPN interface. The private key never leaves this device.
	config := fmt.Sprintf("private_key=%x\nlisten_port=17892\npublic_key=%x\nallowed_ip=10.203.77.2/32\nallowed_ip=fd6a:1142:79::2/128\n", server.Bytes(), client.PublicKey().Bytes())
	if err = wg.IpcSet(config); err == nil {
		err = wg.Up()
	}
	if err != nil {
		wg.Close()
		st.Close()
		return 4
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(clientPrivate)), 32), client.Bytes())
	copy(unsafe.Slice((*byte)(unsafe.Pointer(clientPublic)), 32), client.PublicKey().Bytes())
	copy(unsafe.Slice((*byte)(unsafe.Pointer(serverPublic)), 32), server.PublicKey().Bytes())
	socketPath := socketDir + "/vpn-ready.sock"
	if err = os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		wg.Close()
		st.Close()
		return 5
	}
	hl, err := net.Listen("unix", socketPath)
	if err != nil {
		wg.Close()
		st.Close()
		return 5
	}
	healthListener = hl
	go serveHealth(hl, wg)
	localWG = wg
	localStack = st
	return 0
}

//export TVLocalWGStop
func TVLocalWGStop() {
	if healthListener != nil {
		healthListener.Close()
		healthListener = nil
	}
	if localWG != nil {
		localWG.Close()
		localWG = nil
	}
	if localStack != nil {
		localStack.Close()
		localStack = nil
	}
}

func serveHealth(hl net.Listener, wg *device.Device) {
	for {
		client, err := hl.Accept()
		if err != nil {
			return
		}
		_ = client.SetDeadline(time.Now().Add(time.Second))
		state := byte(0)
		stats, err := wg.IpcGet()
		if err == nil {
			for _, line := range strings.Split(stats, "\n") {
				if value, found := strings.CutPrefix(line, "last_handshake_time_sec="); found && value != "0" {
					state = 1
					break
				}
			}
		}
		_, _ = client.Write([]byte{state})
		client.Close()
	}
}
