package main

import (
	"bufio"
	"encoding/binary"
	"io"
)

type Transport interface {
	Peek() (*Frame, error)
	Read() (*Frame, error)
	Write(*Frame) error
	Close() error
}

func NewTransport(rwc io.ReadWriteCloser) Transport {
	return &transport{
		b:  bufio.NewReader(rwc),
		wc: rwc,
	}
}

type transport struct {
	b  *bufio.Reader
	wc io.WriteCloser
}

func (t *transport) Peek() (*Frame, error) {
	b, err := t.b.Peek(4)
	if err != nil {
		return nil, err
	}

	n := binary.BigEndian.Uint16(b[2:])
	if n > 0 {
		b, err = t.b.Peek(int(n) + 4)
		if err != nil {
			return nil, err
		}
	}

	f := &Frame{}
	_, err = f.Decode(b)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (t *transport) Read() (*Frame, error) {
	var b = make([]byte, 4)
	_, err := io.ReadFull(t.b, b)
	if err != nil {
		return nil, err
	}

	f := &Frame{
		Id:  binary.BigEndian.Uint16(b),
		Len: binary.BigEndian.Uint16(b[2:]),
	}

	f.Data = make([]byte, f.Len)
	_, err = io.ReadFull(t.b, f.Data)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (t *transport) Write(f *Frame) error {
	_, err := t.wc.Write(f.Encode())
	return err
}

func (t *transport) Close() error {
	return t.wc.Close()
}
