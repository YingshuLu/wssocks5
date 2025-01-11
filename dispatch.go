package main

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
)

func NewProxyDispatcher(t Transport) Dispatcher {
	d := &ProxyDispatcher{
		Transport: t,
		RWMutex:   new(sync.RWMutex),
		tunnels:   make(map[uint16]*tunnel),
		acceptCh:  make(chan *Frame, 64),
	}
	go d.run()
	return d
}

type ProxyDispatcher struct {
	Transport
	*sync.RWMutex
	tunnels  map[uint16]*tunnel
	acceptCh chan *Frame
	index    uint16
	closed   atomic.Bool
}

func (d *ProxyDispatcher) run() {
	defer d.Close()

	for {
		f, err := d.Read()
		if err != nil {
			log.Errorf("dispatch read error %v", err)
			break
		}

		log.Debugf("dispatch read frame %d", f.Id)
		t := d.getTunnel(f.Id)
		if t != nil {
			t.readCh <- f
		} else {
			if f.Len == 0 {
				continue
			}
			d.acceptCh <- f
		}
	}

	d.acceptCh <- nil
}

func (d *ProxyDispatcher) IsAlive() bool {
	return !d.closed.Load()
}

func (d *ProxyDispatcher) OpenTunnel(ctx context.Context) (Tunnel, error) {
	count := 0
	for {
		d.Lock()
		id := d.index
		if d.tunnels[id] == nil {
			t := newTunnel(ctx, id, d)
			d.tunnels[id] = t
			d.Unlock()
			return d.tunnels[id], nil
		}
		d.index++
		d.Unlock()

		count++
		if count == 65536 {
			break
		}
	}
	return nil, errors.New("no available tunnel")
}

func (d *ProxyDispatcher) AcceptTunnel(ctx context.Context) (Tunnel, error) {
	select {
	case f := <-d.acceptCh:
		if f == nil {
			return nil, errors.New("WebSocket Connection Closed")
		}

		r, err := ParseMethodRequest(f.Data)
		if err != nil {
			return nil, err
		}
		if len(f.Data) != 2+len(r.Methods) {
			return nil, errors.New("accept not socks5 method request")
		}

		log.Debugf("dispatch accept %d tunnel success", f.Id)
		t := newTunnel(ctx, f.Id, d)
		d.addTunnel(t)
		t.readCh <- f
		return t, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (d *ProxyDispatcher) addTunnel(t *tunnel) {
	d.Lock()
	defer d.Unlock()
	d.tunnels[t.id] = t
}

func (d *ProxyDispatcher) getTunnel(id uint16) *tunnel {
	d.RLock()
	defer d.RUnlock()
	return d.tunnels[id]
}

func (d *ProxyDispatcher) GetTunnel(id uint16) Tunnel {
	return d.getTunnel(id)
}

func (d *ProxyDispatcher) CloseTunnel(id uint16) error {
	d.Lock()
	delete(d.tunnels, id)
	d.Unlock()
	return nil
}

func (d *ProxyDispatcher) Close() error {
	defer func() {
		d.closed.Store(true)
	}()

	for id := range d.tunnels {
		d.CloseTunnel(id)
	}
	return d.Transport.Close()
}

func NewProxyConnection(src, dst io.ReadWriteCloser) *ProxyConnection {
	return &ProxyConnection{
		src: src,
		dst: dst,
	}
}

type ProxyConnection struct {
	src, dst io.ReadWriteCloser
}

func (c *ProxyConnection) TunnelTraffic() {
	go func() {
		defer c.Close()
		_, err := io.Copy(c.dst, c.src)
		if err != nil {
			return
		}
	}()

	go func() {
		defer c.Close()
		_, err := io.Copy(c.src, c.dst)
		if err != nil {
			return
		}
	}()
}

func (c *ProxyConnection) Close() error {
	c.dst.Close()
	return c.src.Close()
}
