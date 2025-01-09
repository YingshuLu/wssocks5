package main

import (
	"encoding/binary"
	"errors"
	"io"
	"sync/atomic"
)

type Frame struct {
	Id   uint16
	Len  uint16
	Data []byte
}

func (f *Frame) Encode() []byte {
	buffer := make([]byte, 4)
	binary.BigEndian.PutUint16(buffer, f.Id)
	binary.BigEndian.PutUint16(buffer[2:], f.Len)
	return append(buffer, f.Data...)
}

func (f *Frame) Decode(data []byte) (int, error) {
	if len(data) < 4 {
		return 0, errors.New("decode Frame need more data")
	}

	f.Id = binary.BigEndian.Uint16(data)
	f.Len = binary.BigEndian.Uint16(data[2:])

	if len(data) < 4+int(f.Len) {
		return 0, errors.New("decode Frame need more data")
	}
	f.Data = data[4 : 4+f.Len]

	return 4 + int(f.Len), nil
}

func (f *Frame) BytesCount() int {
	return 4 + int(f.Len)
}

type Dispatcher interface {
	Transport

	OpenTunnel() (Tunnel, error)

	AcceptTunnel() (Tunnel, error)

	GetTunnel(uint16) Tunnel

	CloseTunnel(uint16) error
}

type Tunnel interface {
	io.ReadWriteCloser

	Id() uint16

	ReadOut() ([]byte, error)
}

type tunnel struct {
	d      Dispatcher
	id     uint16
	closed atomic.Bool
}

func (t *tunnel) Id() uint16 {
	return t.id
}

func (t *tunnel) Read(b []byte) (int, error) {
	frame, err := t.d.Read()
	if err != nil {
		return 0, err
	}
	if frame.Len == 0 {
		return 0, io.EOF
	}
	return copy(b, frame.Data), nil
}

func (t *tunnel) ReadOut() ([]byte, error) {
	frame, err := t.d.Read()
	if err != nil {
		return nil, err
	}

	if frame.Len == 0 {
		return nil, io.EOF
	}
	return frame.Data, nil
}

func (t *tunnel) Write(b []byte) (int, error) {
	frame := &Frame{
		Id:   t.id,
		Len:  uint16(len(b)),
		Data: b,
	}
	return len(b), t.d.Write(frame)
}

func (t *tunnel) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return t.d.CloseTunnel(t.id)
	}
	return nil
}
