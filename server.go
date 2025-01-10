package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
)

type Server struct {
	listenUrl string
	ws        *websocket.Upgrader
}

func (s *Server) Serve() error {
	return s.RunWs()
}

func (s *Server) RegisterWs(path string, mux *http.ServeMux) {
	mux.HandleFunc(path, s.wsAccept)
}

func (s *Server) RunWs() error {
	u, err := url.Parse(s.listenUrl)
	if err != nil {
		return err
	}

	port := u.Port()
	if len(port) == 0 {
		switch u.Scheme {
		case "ws":
			port = ":80"
		case "wss":
			port = ":443"
		default:
			return fmt.Errorf("url %s is invalid ", s.listenUrl)
		}
	} else {
		port = ":" + port
	}

	hs := http.NewServeMux()
	s.RegisterWs(u.Path, hs)
	return http.ListenAndServe(port, hs)
}

func (s *Server) wsAccept(w http.ResponseWriter, r *http.Request) {
	wsc, err := s.ws.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	var rwc = NewWebSocket(wsc)
	var t = NewTransport(rwc)
	p := NewWsSocks5Proxy(context.Background(), NewProxyDispatcher(t))
	go p.Serve()
}
