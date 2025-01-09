package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

const Socks5Version byte = 0x05

const (
	IPV4   byte = 0x01
	DOMAIN byte = 0x03
	IPV6   byte = 0x04
)

const (
	CONNECT byte = 0x01
	BIND    byte = 0x02
	UDP     byte = 0x03
)

const (
	NOAUTH byte = 0x00
	GSSAPI byte = 0x01
	UPASSW byte = 0x02
	IANA   byte = 0x03
	RESV   byte = 0x80
	NOACPT byte = 0xFF
)

const (
	SUCCEEDED byte = 0x00
	GENERAL   byte = 0x01
	NOTALLOW  byte = 0x02
	UNREACH   byte = 0x03
	HUNREACH  byte = 0x04
	REFUSED   byte = 0x05
	TTLEXPIRE byte = 0x06
	CMDNSUPP  byte = 0x07
	ATYPNSUPP byte = 0x08
)

type MethodRequest struct {
	Ver      uint8
	NMethods uint8
	Methods  []uint8
}

func (r *MethodRequest) Encode() []byte {
	return append([]byte{Socks5Version, r.NMethods}, r.Methods...)
}

type MethodResponse struct {
	Ver    uint8
	Method uint8
}

func (r *MethodResponse) Encode() []byte {
	return []byte{Socks5Version, r.Method}
}

type message struct {
	Ver      uint8
	CmdOrRep uint8
	Rsv      uint8
	Atyp     uint8
	Addr     []byte
	Port     uint16
}

func (m *message) Encode() []byte {
	buffer := []byte{Socks5Version, m.CmdOrRep, m.Rsv, m.Atyp}
	buffer = append(buffer, m.Addr...)
	buffer = binary.BigEndian.AppendUint16(buffer, m.Port)
	return buffer
}

func (m *message) Domain() string {
	if m.Atyp != DOMAIN {
		return ""
	}
	return string(m.Addr[1:])
}

func (m *message) IPAddress() *net.IPAddr {
	if (m.Atyp != IPV4 && m.Atyp != IPV6) ||
		len(m.Addr) != net.IPv4len && len(m.Addr) != net.IPv6len {
		return nil
	}
	return &net.IPAddr{IP: m.Addr}
}

func (m *message) Address() string {
	var addr string
	switch m.Atyp {
	case IPV4:
		addr = m.IPAddress().String()
	case IPV6:
		addr = fmt.Sprintf("[%s]", m.IPAddress().String())
	case DOMAIN:
		addr = m.Domain()
	}
	return fmt.Sprintf("%s:%d", addr, m.Port)
}

type Request = message

type Reply = message

func ParseMethodRequest(data []byte) (*MethodRequest, error) {
	if len(data) < 3 {
		return nil, errors.New("method request need more data")
	}
	return &MethodRequest{
		Ver:      data[0],
		NMethods: data[1],
		Methods:  data[2:],
	}, nil
}

func ParseMethodResponse(data []byte) (*MethodResponse, error) {
	if len(data) < 2 {
		return nil, errors.New("method response need more data")
	}
	return &MethodResponse{
		Ver:    data[0],
		Method: data[1],
	}, nil
}

func parseMessage(data []byte) (*message, error) {
	if len(data) < 10 {
		return nil, errors.New("message need more data")
	}
	return &message{
		Ver:      data[0],
		CmdOrRep: data[1],
		Rsv:      data[2],
		Atyp:     data[3],
		Addr:     data[4 : len(data)-2],
		Port:     binary.BigEndian.Uint16(data[len(data)-2:]),
	}, nil
}

func ParseRequest(data []byte) (*Request, error) {
	return parseMessage(data)
}

func ParseReply(data []byte) (*Reply, error) {
	return parseMessage(data)
}
