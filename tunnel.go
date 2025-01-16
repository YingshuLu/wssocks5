package main

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
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

	IsAlive() bool

	OpenTunnel(context.Context) (Tunnel, error)

	AcceptTunnel(context.Context) (Tunnel, error)

	GetTunnel(uint16) Tunnel

	CloseTunnel(uint16) error
}

type Tunnel interface {
	io.ReadWriteCloser

	ReadOut() ([]byte, error)
}

func newTunnel(ctx context.Context, id uint16, d Dispatcher) *tunnel {
	return &tunnel{
		ctx:    ctx,
		d:      d,
		readCh: make(chan *Frame, 64),
		id:     id,
		closed: &atomic.Bool{},
		eof:    &atomic.Bool{},
	}
}

type tunnel struct {
	ctx    context.Context
	d      Dispatcher
	readCh chan *Frame
	id     uint16
	closed *atomic.Bool
	buffer []byte
	eof    *atomic.Bool
}

func (t *tunnel) Id() uint16 {
	return t.id
}

func (t *tunnel) readFrame() (*Frame, error) {
	if t.eof.Load() {
		log.Errorf("tunnel %d read eof", t.id)
		return nil, io.EOF
	}

	select {
	case frame := <-t.readCh:
		if frame.Len == 0 {
			t.eof.Store(true)
			return nil, io.EOF
		}
		return frame, nil

	case <-t.ctx.Done():
		return nil, t.ctx.Err()
	}
}

func (t *tunnel) Read(b []byte) (int, error) {
	var n int

	if len(t.buffer) > 0 {
		n = copy(b, t.buffer)
		t.buffer = t.buffer[n:]
		if len(t.buffer) == 0 {
			t.buffer = nil
		}
		return n, nil
	}

	frame, err := t.readFrame()
	if err != nil {
		return 0, err
	}

	n = copy(b, frame.Data)
	if n < len(frame.Data) {
		t.buffer = frame.Data[n:]
	}
	return n, nil
}

func (t *tunnel) ReadOut() ([]byte, error) {
	if len(t.buffer) > 0 {
		d := t.buffer
		t.buffer = nil
		return d, nil
	}

	frame, err := t.readFrame()
	if err != nil {
		return nil, err
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
	if t.closed.CompareAndSwap(false, true) {
		// should send FIN at first before removing the tunnel
		defer t.d.CloseTunnel(t.id)
		return t.notifyClose()
	}
	return nil
}

func (t *tunnel) notifyClose() error {
	if t.eof.CompareAndSwap(false, true) {
		return t.sendFinFrame()
	}
	return nil
}

func (t *tunnel) sendFinFrame() error {
	_, err := t.Write(nil)
	return err
}
