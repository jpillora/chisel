package tunnel

import (
	"context"
	"encoding/gob"
	"io"
)

type udpPacket struct {
	Src     string
	Dst     string
	Payload []byte
}

func init() {
	gob.Register(&udpPacket{})
}

type udpOutbound struct {
	r *gob.Decoder
	w *gob.Encoder
	c io.Closer
}

func (o *udpOutbound) encode(src, dst string, b []byte) error {
	return o.w.Encode(udpPacket{
		Src:     src,
		Dst:     dst,
		Payload: b,
	})
}

func (o *udpOutbound) decode(p *udpPacket) error {
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
