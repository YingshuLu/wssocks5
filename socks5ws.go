package main

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type NewDispatcher = func() (Dispatcher, error)

func NewSocks5WsProxy(ctx context.Context, newDispatcher NewDispatcher, l net.Listener) *Socks5WsProxy {
	d, err := newDispatcher()
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(ctx)
	return &Socks5WsProxy{
		Mutex:            &sync.Mutex{},
		Dispatcher:       d,
		Listener:         l,
		ctx:              ctx,
		cancel:           cancel,
		createDispatcher: newDispatcher,
	}
}

type Socks5WsProxy struct {
	*sync.Mutex
	Dispatcher
	net.Listener
	ctx              context.Context
	cancel           context.CancelFunc
	createDispatcher NewDispatcher
	reconnect        int
}

func (p *Socks5WsProxy) Serve() error {
	for {
		conn, err := p.Accept()
		if err != nil {
			return err
		}
		go p.accept(conn)
	}
}

func (p *Socks5WsProxy) Close() error {
	p.cancel()
	p.Dispatcher.Close()
	return p.Listener.Close()
}

func (p *Socks5WsProxy) accept(conn net.Conn) {
	tunnel, err := p.handshake(conn)
	if err != nil {
		log.Errorf("client socks5 handshake error: %v", err)
		if tunnel != nil {
			tunnel.Close()
		}
		conn.Close()
		return
	}
	log.Info("client socks5 handshake success")

	var proxyConnection = NewProxyConnection(tunnel, conn)
	proxyConnection.TunnelTraffic()
}

func (p *Socks5WsProxy) handshake(conn net.Conn) (s io.ReadWriteCloser, err error) {
	if !p.Dispatcher.IsAlive() {
		err = func() error {
			p.Lock()
			defer p.Unlock()
			if p.reconnect >= 3 {
				time.Sleep(8 * time.Second)
			}

			d, err := p.createDispatcher()
			if err != nil {
				p.reconnect += 1
				return err
			}
			p.reconnect = 0
			p.Dispatcher = d
			return nil
		}()
		if err != nil {
			return
		}
	}

	newConn := func(ctx context.Context) (io.ReadWriteCloser, error) {
		return p.OpenTunnel(ctx)
	}

	return ProxyHandshake(p.ctx, conn, newConn)
}
