package cnet

import (
	"fmt"
	"sync/atomic"
)

//ConnCount is a connection counter
type ConnCount struct {
	count int32
	open  int32
}

func (c *ConnCount) New() int32 {
	return atomic.AddInt32(&c.count, 1)
}

func (c *ConnCount) Open() {
	atomic.AddInt32(&c.open, 1)
}

func (c *ConnCount) Close() {
	atomic.AddInt32(&c.open, -1)
}

func (c *ConnCount) String() string {
	return fmt.Sprintf("[%d/%d]", atomic.LoadInt32(&c.open), atomic.LoadInt32(&c.count))
}
