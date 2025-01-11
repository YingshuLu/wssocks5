package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func NewServer(listenUrl string) *Server {
	return &Server{
		listenUrl: listenUrl,
		ws: &websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
	}
}

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
	var tlsEnabled = strings.EqualFold(u.Scheme, "wss")
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

	if tlsEnabled {
		cert, err := GenX509KeyPair(u.Hostname())
		if err != nil {
			return err
		}

		config := &tls.Config{Certificates: []tls.Certificate{cert}}
		srv := &http.Server{
			Addr:      port,
			Handler:   hs,
			TLSConfig: config,
		}
		return srv.ListenAndServeTLS("", "")
	}

	return http.ListenAndServe(port, hs)
}

func (s *Server) wsAccept(w http.ResponseWriter, r *http.Request) {
	if len(args.Secret) > 0 {
		secret := r.Header.Get(AuthToken)
		if !strings.EqualFold(secret, args.Secret) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Auth Failure!"))
			return
		}
	}

	wsc, err := s.ws.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	var rwc = NewWebSocket(wsc)
	var t = NewTransport(rwc)
	p := NewWsSocks5Proxy(context.Background(), NewProxyDispatcher(t))
	go p.Serve()
}

func GenX509KeyPair(domain string) (tls.Certificate, error) {
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.Unix()),
		Subject: pkix.Name{
			CommonName:         domain,
			Country:            []string{"USA"},
			Organization:       []string{domain},
			OrganizationalUnit: []string{"fake"},
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0),
		SubjectKeyId:          []byte{113, 117, 105, 99, 107, 115, 101, 114, 118, 101},
		BasicConstraintsValid: true,
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	cert, err := x509.CreateCertificate(rand.Reader, template, template,
		priv.Public(), priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	var outCert tls.Certificate
	outCert.Certificate = append(outCert.Certificate, cert)
	outCert.PrivateKey = priv

	return outCert, nil
}
