package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
)

func NewClientProxy(listenPort int, wsServerAddr string, ignoreCertificate bool) *ClientProxy {
	return &ClientProxy{
		listenPort:        listenPort,
		serverAddr:        wsServerAddr,
		ignoreCertificate: ignoreCertificate,
		wait:              make(chan bool, 1),
	}
}

type ClientProxy struct {
	listenPort        int
	serverAddr        string
	listener          net.Listener
	proxies           []*Socks5WsProxy
	ignoreCertificate bool
	wait              chan bool
}

func (c *ClientProxy) wsDispatcher() (Dispatcher, error) {
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.ignoreCertificate,
		},
	}

	var requestHeader = http.Header{}
	if len(args.Secret) > 0 {
		requestHeader.Add(AuthToken, args.Secret)
	}

	wsc, _, err := dialer.Dial(c.serverAddr, requestHeader)
	if err != nil {
		return nil, err
	}

	t := NewTransport(NewWebSocket(wsc))
	d := NewProxyDispatcher(t)
	return d, nil
}

func (c *ClientProxy) Serve() error {
	var err error
	listenAddr := fmt.Sprintf(":%d", c.listenPort)
	c.listener, err = net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}

	clientCount := int(math.Max(float64(args.ClientCount), 1))
	c.proxies = make([]*Socks5WsProxy, clientCount)
	for i := 0; i < clientCount; i++ {
		p := NewSocks5WsProxy(context.Background(), c.wsDispatcher, c.listener)
		c.proxies[i] = p
		go p.Serve()
	}
	<-c.wait
	return nil
}

func (c *ClientProxy) Close() error {
	for _, p := range c.proxies {
		p.Close()
	}
	c.wait <- true
	return nil
}
