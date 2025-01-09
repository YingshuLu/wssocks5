package main

import (
	"context"
	"errors"
	"net"
)

func NewSocks5WsProxy(ctx context.Context, l net.Listener) *Socks5WsProxy {
	ctx, cancel := context.WithCancel(ctx)
	return &Socks5WsProxy{
		Listener: l,
		ctx:      ctx,
		cancel:   cancel,
	}
}

type Socks5WsProxy struct {
	Dispatcher
	net.Listener
	ctx    context.Context
	cancel context.CancelFunc
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
		if tunnel != nil {
			tunnel.Close()
		}
		conn.Close()
		return
	}

	var proxyConnection = NewProxyConnection(tunnel, conn)
	proxyConnection.TunnelTraffic()
}

func (p *Socks5WsProxy) handshake(conn net.Conn) (tunnel Tunnel, err error) {
	var (
		n      int
		buffer = make([]byte, 256)
	)

	defer func() {
		if err != nil {
			return
		}
	}()

	n, err = conn.Read(buffer)
	if err != nil {
		return
	}

	_, err = ParseMethodRequest(buffer[:n])
	if err != nil {
		return
	}

	methodReply := &MethodResponse{Socks5Version, NOAUTH}
	_, err = conn.Write(methodReply.Encode())
	if err != nil {
		return
	}

	n, err = conn.Read(buffer)
	req, err := ParseRequest(buffer[:n])
	if err != nil {
		p.sendReply(conn, req, REFUSED)
		return
	}

	tunnel, err = p.OpenTunnel()
	methodRequest := &MethodRequest{Socks5Version, 1, []uint8{NOAUTH}}
	_, err = tunnel.Write(methodRequest.Encode())
	if err != nil {
		p.sendReply(conn, req, REFUSED)
		return
	}

	n, err = tunnel.Read(buffer)
	if err != nil {
		p.sendReply(conn, req, REFUSED)
		return
	}

	_, err = ParseMethodResponse(buffer[:n])
	if err != nil {
		p.sendReply(conn, req, REFUSED)
		return
	}

	_, err = tunnel.Write(req.Encode())
	if err != nil {
		p.sendReply(conn, req, REFUSED)
		return
	}

	n, err = tunnel.Read(buffer)
	if err != nil {
		p.sendReply(conn, req, REFUSED)
		return
	}

	reply, err := ParseReply(buffer[:n])
	if err != nil {
		p.sendReply(conn, req, REFUSED)
		return
	}

	_, err = conn.Write(buffer[:n])
	if err != nil {
		return
	}

	if reply.CmdOrRep != SUCCEEDED {
		err = errors.New("socks5 handshake failed")
		return
	}

	return
}

func (p *Socks5WsProxy) sendReply(conn net.Conn, req *Request, rep byte) error {
	reply := &Reply{
		Ver:      Socks5Version,
		CmdOrRep: rep,
		Atyp:     req.Atyp,
		Addr:     req.Addr,
		Port:     req.Port,
	}
	_, err := conn.Write(reply.Encode())
	return err
}
