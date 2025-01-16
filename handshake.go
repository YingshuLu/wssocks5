package main

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Phase = string

const (
	initPhase          = "Init"
	readMethodRequest  = "ReadMethodRequest"
	parseMethodRequest = "ParseMethodRequest"
	writeMethodReply   = "WriteMethodReply"
	readSocks5Request  = "ReadSocks5Request"
	parseSocks5Request = "ParseSocks5Request"
	newProxyConnection = "NewProxyConnection"
	writeMethodRequest = "WriteMethodRequest"
	readMethodReply    = "ReadMethodReply"
	parseMethodReply   = "ParseMethodReply"
	writeSocks5Request = "WriteSocks5Request"
	readSocks5Reply    = "ReadSocks5Reply"
	parseSocks5Reply   = "ParseSocks5Reply"
	writeSocks5Reply   = "WriteSocks5Reply"
	failureSocks5Reply = "Socks5ReplyFailure"
)

type NewConnection = func(ctx context.Context) (io.ReadWriteCloser, error)

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

func ProxyHandshake(ctx context.Context, c io.ReadWriteCloser, newConn NewConnection) (s io.ReadWriteCloser, err error) {
	var (
		n      int
		buffer = make([]byte, 1024)
		phase  = initPhase
	)

	defer func() {
		if err != nil {
			err = errors.Wrapf(err, "[proxy handshak] error on phase: %s", phase)
			log.Error(err)
			return
		}
	}()

	n, err = c.Read(buffer)
	if err != nil {
		phase = readMethodRequest
		return
	}

	_, err = ParseMethodRequest(buffer[:n])
	if err != nil {
		phase = parseMethodRequest
		return
	}

	methodReply := &MethodReply{Socks5Version, NOAUTH}
	_, err = c.Write(methodReply.Encode())
	if err != nil {
		phase = writeMethodReply
		return
	}

	n, err = c.Read(buffer)
	if err != nil {
		phase = readSocks5Request
		return
	}

	req, err := ParseRequest(buffer[:n])
	if err != nil {
		phase = parseSocks5Request
		SendSocks5Reply(c, req, REFUSED)
		return
	}

	log.Debugf("client - try to tunnel to address %s", req.Address())

	s, err = newConn(ctx)
	if err != nil {
		phase = newProxyConnection
		return
	}
	methodRequest := &MethodRequest{Socks5Version, 1, []uint8{NOAUTH}}
	_, err = s.Write(methodRequest.Encode())
	if err != nil {
		phase = writeMethodRequest
		SendSocks5Reply(c, req, REFUSED)
		return
	}

	n, err = s.Read(buffer)
	if err != nil {
		phase = readMethodReply
		SendSocks5Reply(c, req, REFUSED)
		return
	}

	_, err = ParseMethodReply(buffer[:n])
	if err != nil {
		phase = parseMethodReply
		SendSocks5Reply(c, req, REFUSED)
		return
	}

	_, err = s.Write(req.Encode())
	if err != nil {
		phase = writeSocks5Request
		SendSocks5Reply(c, req, REFUSED)
		return
	}

	n, err = s.Read(buffer)
	if err != nil {
		phase = readSocks5Reply
		SendSocks5Reply(c, req, REFUSED)
		return
	}

	reply, err := ParseReply(buffer[:n])
	if err != nil {
		phase = parseSocks5Reply
		SendSocks5Reply(c, req, REFUSED)
		return
	}

	_, err = c.Write(buffer[:n])
	if err != nil {
		phase = writeSocks5Reply
		return
	}

	if reply.CmdOrRep != SUCCEEDED {
		phase = failureSocks5Reply
		err = fmt.Errorf("socks5 Reply with error: %v", reply.CmdOrRep)
		return
	}

	return
}

func ServerHandshake(c io.ReadWriteCloser) (s io.ReadWriteCloser, err error) {
	var (
		n      int
		buffer = make([]byte, 1024)
		phase  = initPhase
	)

	defer func() {
		if err != nil {
			err = errors.Wrapf(err, "[server handshake] error on phase: %v", phase)
			log.Error(err)
			return
		}
	}()

	n, err = c.Read(buffer)
	if err != nil {
		phase = readMethodRequest
		return
	}

	_, err = ParseMethodRequest(buffer[:n])
	if err != nil {
		phase = parseMethodRequest
		return
	}

	methodReply := &MethodReply{Socks5Version, NOAUTH}
	_, err = c.Write(methodReply.Encode())
	if err != nil {
		phase = writeMethodReply
		return
	}

	n, err = c.Read(buffer)
	if err != nil {
		phase = readSocks5Request
		return
	}

	req, err := ParseRequest(buffer[:n])
	if err != nil {
		phase = parseSocks5Request
		SendSocks5Reply(c, req, REFUSED)
		return
	}

	network := "tcp"
	if req.CmdOrRep == UDP {
		network = "udp"
	}

	log.Debugf("server - try to connect %s://%s", network, req.Address())

	s, err = net.Dial(network, req.Address())
	if err != nil {
		phase = fmt.Sprintf("connect to Remote %s://%s", network, req.Address())
		SendSocks5Reply(c, req, UNREACH)
		return
	}

	err = SendSocks5Reply(c, req, SUCCEEDED)
	if err != nil {
		phase = writeSocks5Reply
		return
	}
	return
}
