package main

import (
	"context"
	"io"
)

type WsSocks5Proxy struct {
	Dispatcher
	ctx    context.Context
	cancel context.CancelFunc
}

func NewWsSocks5Proxy(ctx context.Context, d Dispatcher) *WsSocks5Proxy {
	ctx, cancel := context.WithCancel(ctx)
	return &WsSocks5Proxy{
		Dispatcher: d,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (w *WsSocks5Proxy) Serve() error {
	for {
		tunnel, err := w.AcceptTunnel(w.ctx)
		if err != nil {
			return err
		}
		go w.accept(tunnel)
	}
}

func (w *WsSocks5Proxy) Close() error {
	w.cancel()
	return w.Dispatcher.Close()
}

func (w *WsSocks5Proxy) accept(tunnel Tunnel) {
	target, err := w.handshake(tunnel)
	if err != nil {
		if target != nil {
			target.Close()
		}
		tunnel.Close()
		return
	}

	var proxyConnection = NewProxyConnection(tunnel, target)
	proxyConnection.TunnelTraffic()
}

func (w *WsSocks5Proxy) handshake(tunnel Tunnel) (io.ReadWriteCloser, error) {
	return ServerHandshake(tunnel)
}
