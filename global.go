package main

import (
	"io"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.ErrorLevel)
}

const (
	AuthToken = "X-Auth-Token"
)

type Args struct {
	Mode        string `arg:"required"`
	Secret      string
	ClientCount int
	ServerUrl   string
	ListenPort  int
	Verbose     bool
}

var args = &Args{}

func SendSocks5Reply(w io.Writer, req *Request, rep byte) error {
	reply := &Reply{
		Ver:      Socks5Version,
		CmdOrRep: rep,
	}
	if req != nil {
		reply.Atyp = req.Atyp
		reply.Addr = req.Addr
		reply.Port = req.Port
	}
	_, err := w.Write(reply.Encode())
	return err
}
