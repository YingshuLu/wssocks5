package main

import (
	"context"
	"net"
)

type WsSocks5Proxy struct {
	Dispatcher
	ctx    context.Context
	cancel context.CancelFunc
}

func NewWsSocks5Proxy(d Dispatcher) *WsSocks5Proxy {
	ctx, cancel := context.WithCancel(context.Background())
	return &WsSocks5Proxy{
		Dispatcher: d,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (w *WsSocks5Proxy) Serve() {
	for {
		tunnel, err := w.AcceptTunnel()
		if err != nil {
			break
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
	defer func() {
		if err != nil {
			return
		}
	}()

	data, err := tunnel.ReadOut()
	if err != nil {
		return
	}

	_, err = ParseMethodRequest(data)
	if err != nil {
		return
	}

	methodReply := &MethodResponse{Socks5Version, NOAUTH}
	_, err = tunnel.Write(methodReply.Encode())
	if err != nil {
		return
	}

	data, err = tunnel.ReadOut()
	req, err := ParseRequest(data)
	if err != nil {
		w.sendReply(tunnel, req, REFUSED)
		return
	}

	network := "tcp"
	if req.CmdOrRep == UDP {
		network = "udp"
	}
	target, err = net.Dial(network, req.Address())
	if err != nil {
		w.sendReply(tunnel, req, UNREACH)
		return
	}
	return
}

func (w *WsSocks5Proxy) sendReply(tunnel Tunnel, req *Request, rep byte) error {
	reply := &Reply{
		Ver:      Socks5Version,
		CmdOrRep: rep,
		Atyp:     req.Atyp,
		Addr:     req.Addr,
		Port:     req.Port,
	}
	_, err := tunnel.Write(reply.Encode())
	return err
}
