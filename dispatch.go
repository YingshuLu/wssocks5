package main

import (
	"errors"
	"io"
	"sync"
)

func NewProxyDispatcher(t Transport) Dispatcher {
	return &ProxyDispatcher{
		Transport: t,
		RWMutex:   new(sync.RWMutex),
		tunnels:   make(map[uint16]Tunnel),
	}
}

type ProxyDispatcher struct {
	Transport
	*sync.RWMutex
	tunnels map[uint16]Tunnel
	index   uint16
}

func (d *ProxyDispatcher) OpenTunnel() (Tunnel, error) {
	count := 0
	for {
		d.Lock()
		id := d.index
		if d.tunnels[id] == nil {
			t := &tunnel{d: d, id: id}
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

func (d *ProxyDispatcher) AcceptTunnel() (Tunnel, error) {
	tunnelCh := make(chan any, 1)

	go func() {
		for {
			f, err := d.Peek()
			if err != nil {
				tunnelCh <- err
				break
			}

			t := d.GetTunnel(f.Id)
			if t != nil {
				continue
			}

			t = &tunnel{d: d, id: f.Id}
			d.addTunnel(t)
			tunnelCh <- t
			break
		}
	}()

	r := <-tunnelCh
	switch v := r.(type) {
	case error:
		return nil, v
	case Tunnel:
		return v, nil
	default:
		return nil, errors.New("unknown error")
	}
}

func (d *ProxyDispatcher) addTunnel(t Tunnel) {
	d.Lock()
	defer d.Unlock()
	d.tunnels[t.Id()] = t
}

func (d *ProxyDispatcher) GetTunnel(id uint16) Tunnel {
	d.RLock()
	defer d.RUnlock()
	return d.tunnels[id]
}

func (d *ProxyDispatcher) CloseTunnel(id uint16) error {
	var t Tunnel
	defer func() {
		if t != nil {
			d.Lock()
			delete(d.tunnels, id)
			d.Unlock()
		}
	}()

	t = d.GetTunnel(id)
	if t != nil {
		return d.Transport.Write(&Frame{Id: t.Id()})
	}
	return nil
}

func (d *ProxyDispatcher) Close() error {
	d.Lock()
	defer d.Unlock()
	for id := range d.tunnels {
		d.CloseTunnel(id)
	}
	return d.Transport.Close()
}

func NewProxyConnection(t Tunnel, peer io.ReadWriteCloser) *ProxyConnection {
	return &ProxyConnection{
		tunnel: t,
		peer:   peer,
	}
}

type ProxyConnection struct {
	tunnel Tunnel
	peer   io.ReadWriteCloser
}

func (c *ProxyConnection) TunnelTraffic() {
	go func() {
		defer c.Close()

		var (
			data []byte
			err  error
		)
		for {
			data, err = c.tunnel.ReadOut()
			if err != nil {
				break
			}
			c.peer.Write(data)
		}
	}()

	go func() {
		defer c.Close()

		data := make([]byte, 2048)
		var (
			n   int
			err error
		)
		for {
			n, err = c.peer.Read(data)
			if err != nil {
				break
			}
			c.tunnel.Write(data[:n])
		}
	}()
}

func (c *ProxyConnection) Close() error {
	c.peer.Close()
	return c.tunnel.Close()
}
