package chshare

import (
	"fmt"
	"sync/atomic"
)

type ConnStats struct {
	count int32
	open  int32
}

func (c *ConnStats) New() int32 {
	return atomic.AddInt32(&c.count, 1)
}

func (c *ConnStats) Open() {
	atomic.AddInt32(&c.open, 1)
}

func (c *ConnStats) Close() {
	atomic.AddInt32(&c.open, -1)
}

func (c *ConnStats) String() string {
	return fmt.Sprintf("[%d/%d]", atomic.LoadInt32(&c.open), atomic.LoadInt32(&c.count))
}
