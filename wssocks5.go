package main

import (
	"context"
	"fmt"
	"net"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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

func (w *WsSocks5Proxy) handshake(tunnel Tunnel) (target net.Conn, err error) {
	phase := "init"
	defer func() {
		if err != nil {
			err = errors.Wrapf(err, "[handshake] error on phase: %v", phase)
			return
		}
	}()

	data, err := tunnel.ReadOut()
	if err != nil {
		phase = "read MethodRequest"
		return
	}

	_, err = ParseMethodRequest(data)
	if err != nil {
		phase = "parse MethodRequest"
		return
	}

	methodReply := &MethodReply{Socks5Version, NOAUTH}
	_, err = tunnel.Write(methodReply.Encode())
	if err != nil {
		phase = "write MethodReply"
		return
	}

	data, err = tunnel.ReadOut()
	if err != nil {
		phase = "read Request"
		return
	}

	req, err := ParseRequest(data)
	if err != nil {
		phase = "parse Request"
		SendSocks5Reply(tunnel, req, REFUSED)
		return
	}

	network := "tcp"
	if req.CmdOrRep == UDP {
		network = "udp"
	}

	log.Debugf("server - try to connect %s://%s", network, req.Address())

	target, err = net.Dial(network, req.Address())
	if err != nil {
		phase = fmt.Sprintf("connect to Remote %s://%s", network, req.Address())
		SendSocks5Reply(tunnel, req, UNREACH)
		return
	}

	err = SendSocks5Reply(tunnel, req, SUCCEEDED)
	if err != nil {
		phase = "write Reply"
		return
	}
	return
}
