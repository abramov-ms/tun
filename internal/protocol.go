package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

type MessageKind uint8

const (
	Data MessageKind = iota
	HeartBeat
	EndOfStream
)

type Message struct {
	Kind    MessageKind
	Payload []byte
}

func ReadMessage(r io.Reader) (*Message, error) {
	var kindBuffer [1]byte
	_, err := r.Read(kindBuffer[:])
	if err != nil {
		return nil, err
	}

	kind := MessageKind(kindBuffer[0])
	if kind != Data && kind != HeartBeat && kind != EndOfStream {
		return nil, fmt.Errorf("unknown message kind")
	}

	if kind == HeartBeat || kind == EndOfStream {
		return &Message{Kind: kind}, nil
	}

	var sizeBuffer [8]byte
	_, err = r.Read(sizeBuffer[:])
	if err != nil {
		return nil, err
	}

	size := binary.LittleEndian.Uint64(sizeBuffer[:])
	payload := make([]byte, size)
	done := uint64(0)
	for done < size {
		bytes, err := r.Read(payload[done:])
		if err != nil {
			return nil, err
		}

		done += uint64(bytes)
	}

	return &Message{Kind: kind, Payload: payload}, nil
}

func writeAll(w io.Writer, buffer []byte) error {
	done := 0
	for done < len(buffer) {
		bytes, err := w.Write(buffer[done:])
		if err != nil {
			return err
		}

		done += bytes
	}

	return nil
}

func (m *Message) Write(w io.Writer) error {
	kindBuffer := [1]byte{byte(m.Kind)}
	if err := writeAll(w, kindBuffer[:]); err != nil {
		return err
	}

	if m.Payload == nil {
		return nil
	}

	var sizeBuffer [8]byte
	binary.LittleEndian.PutUint64(sizeBuffer[:], uint64(len(m.Payload)))
	if err := writeAll(w, sizeBuffer[:]); err != nil {
		return err
	}

	return writeAll(w, m.Payload)
}
