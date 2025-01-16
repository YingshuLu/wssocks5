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
		acceptCh:  make(chan *tunnel, 64),
	}
	go d.run()
	return d
}

type ProxyDispatcher struct {
	Transport
	*sync.RWMutex
	tunnels  map[uint16]*tunnel
	acceptCh chan *tunnel
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
		if t == nil {
			if f.Len == 0 {
				log.Warnf("tunnel %d closed, get the late EOF", f.Id)
				continue
			}

			r, err := ParseMethodRequest(f.Data)
			if err != nil {
				log.Errorf("websocket accept tunnel %d, failure with parse MethodRequest: %v", f.Id, err)
				continue
			}

			if len(f.Data) != 2+len(r.Methods) {
				log.Errorf("accept frame %d not socks5 method request", f.Id)
				continue
			}

			log.Debugf("dispatch accept %d tunnel success", f.Id)
			t = newTunnel(context.TODO(), f.Id, d)
			d.addTunnel(t)
			d.acceptCh <- t
		}
		t.readCh <- f
	}

	d.acceptCh <- nil
}

func (d *ProxyDispatcher) IsAlive() bool {
	return !d.closed.Load()
}

func (d *ProxyDispatcher) OpenTunnel(ctx context.Context) (Tunnel, error) {
	d.Lock()
	defer d.Unlock()

	count := 0
	for {
		id := d.index
		if d.tunnels[id] == nil {
			t := newTunnel(ctx, id, d)
			d.tunnels[id] = t
			return d.tunnels[id], nil
		}
		d.index++

		count++
		if count == 65536 {
			break
		}
	}
	return nil, errors.New("no available tunnel")
}

func (d *ProxyDispatcher) AcceptTunnel(ctx context.Context) (Tunnel, error) {
	select {
	case t := <-d.acceptCh:
		t.ctx = ctx
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
