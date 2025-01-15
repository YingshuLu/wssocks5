package main

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
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

func (p *Socks5WsProxy) handshake(conn net.Conn) (tunnel Tunnel, err error) {
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

	var (
		n      int
		buffer = make([]byte, 256)
	)

	phase := "init"
	defer func() {
		if err != nil {
			err = errors.Wrapf(err, "[handshak] error on phase: %s", phase)
			return
		}
	}()

	n, err = conn.Read(buffer)
	if err != nil {
		phase = "read MethodRequest"
		return
	}

	_, err = ParseMethodRequest(buffer[:n])
	if err != nil {
		phase = "parse MethodRequest"
		return
	}

	methodReply := &MethodReply{Socks5Version, NOAUTH}
	_, err = conn.Write(methodReply.Encode())
	if err != nil {
		phase = "write MethodReply"
		return
	}

	n, err = conn.Read(buffer)
	if err != nil {
		phase = "read Request"
		return
	}

	req, err := ParseRequest(buffer[:n])
	if err != nil {
		phase = "parse Request"
		SendSocks5Reply(conn, req, REFUSED)
		return
	}

	log.Debugf("client - try to tunnel to address %s", req.Address())

	tunnel, err = p.OpenTunnel(p.ctx)
	methodRequest := &MethodRequest{Socks5Version, 1, []uint8{NOAUTH}}
	_, err = tunnel.Write(methodRequest.Encode())
	if err != nil {
		phase = "write MethodRequest"
		SendSocks5Reply(conn, req, REFUSED)
		return
	}

	n, err = tunnel.Read(buffer)
	if err != nil {
		phase = "read MethodReply"
		SendSocks5Reply(conn, req, REFUSED)
		return
	}

	_, err = ParseMethodReply(buffer[:n])
	if err != nil {
		phase = "parse MethodReply"
		SendSocks5Reply(conn, req, REFUSED)
		return
	}

	_, err = tunnel.Write(req.Encode())
	if err != nil {
		phase = "write Request"
		SendSocks5Reply(conn, req, REFUSED)
		return
	}

	n, err = tunnel.Read(buffer)
	if err != nil {
		phase = "read Reply"
		SendSocks5Reply(conn, req, REFUSED)
		return
	}

	reply, err := ParseReply(buffer[:n])
	if err != nil {
		phase = "parse Reply"
		SendSocks5Reply(conn, req, REFUSED)
		return
	}

	_, err = conn.Write(buffer[:n])
	if err != nil {
		phase = "write Reply"
		return
	}

	if reply.CmdOrRep != SUCCEEDED {
		phase = "failure Reply"
		err = fmt.Errorf("socks5 Reply with error: %v", reply.CmdOrRep)
		return
	}

	return
}
