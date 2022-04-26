package tunnel

import (
	"context"
	"encoding/gob"
	"io"
)

type udpPacket struct {
	Src     string
	Payload []byte
}

func init() {
	gob.Register(&udpPacket{})
}

//udpChannel encodes/decodes udp payloads over a stream
type udpChannel struct {
	r *gob.Decoder
	w *gob.Encoder
	c io.Closer
}

func (o *udpChannel) encode(src string, b []byte) error {
	return o.w.Encode(udpPacket{
		Src:     src,
		Payload: b,
	})
}

func (o *udpChannel) decode(p *udpPacket) error {
	return o.r.Decode(p)
}

func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
