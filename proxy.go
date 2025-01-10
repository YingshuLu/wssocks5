package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/gorilla/websocket"
)

func NewClientProxy(listenPort int, wsServerAddr string, ignoreCertificate bool) *ClientProxy {
	return &ClientProxy{
		listenPort:        listenPort,
		serverAddr:        wsServerAddr,
		ignoreCertificate: ignoreCertificate,
	}
}

type ClientProxy struct {
	listenPort        int
	serverAddr        string
	listener          net.Listener
	proxy             *Socks5WsProxy
	ignoreCertificate bool
}

func (c *ClientProxy) wsDispatcher() (Dispatcher, error) {
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.ignoreCertificate,
		},
	}

	wsc, _, err := dialer.Dial(c.serverAddr, nil)
	if err != nil {
		return nil, err
	}

	t := NewTransport(NewWebSocket(wsc))
	d := NewProxyDispatcher(t)
	return d, nil
}

func (c *ClientProxy) Serve() error {
	var err error
	c.proxy = NewSocks5WsProxy(context.Background(), c.wsDispatcher, c.listener)
	listenAddr := fmt.Sprintf(":%d", c.listenPort)
	c.listener, err = net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	return c.proxy.Serve()
}

func (c *ClientProxy) Close() error {
	return c.proxy.Close()
}
