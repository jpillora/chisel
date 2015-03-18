package chshare

// overview: only half the result is used as the output
// next -> sha512(next) -> [output|next] ->

import (
	"crypto/sha512"
	"io"
)

const DetermRandIter = 1024

func NewDetermRand(seed []byte) io.Reader {
	var out []byte
	//strengthen seed
	var next = seed
	for i := 0; i < DetermRandIter; i++ {
		next, out = hash(next)
	}
	return &DetermRand{
		next: next,
		out:  out,
	}
}

type DetermRand struct {
	next, out []byte
}

func (d *DetermRand) Read(b []byte) (int, error) {
	n := 0
	l := len(b)
	for n < l {
		next, out := hash(d.next)
		n += copy(b[n:], out)
		d.next = next
	}
	return n, nil
}

func hash(input []byte) ([]byte, []byte) {
	nextout := sha512.Sum512(input)
	return nextout[:sha512.Size/2], nextout[sha512.Size/2:]
}
