package cnet

import (
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/sizestr"
)

//NewMeter to measure readers/writers
func NewMeter(l *cio.Logger) *Meter {
	return &Meter{l: l}
}

//Meter can be inserted in the path or
//of a reader or writer to measure the
//throughput
type Meter struct {
	//meter state
	sent, recv int64
	//print state
	l            *cio.Logger
	printing     uint32
	last         int64
	lsent, lrecv int64
}

func (m *Meter) print() {
	//move out of the read/write path asap
	if atomic.CompareAndSwapUint32(&m.printing, 0, 1) {
		go m.goprint()
	}
}

func (m *Meter) goprint() {
	time.Sleep(time.Second)
	//snapshot
	s := atomic.LoadInt64(&m.sent)
	r := atomic.LoadInt64(&m.recv)
	//compute speed
	curr := time.Now().UnixNano()
	last := atomic.LoadInt64(&m.last)
	dt := time.Duration(curr-last) * time.Nanosecond
	ls := atomic.LoadInt64(&m.lsent)
	lr := atomic.LoadInt64(&m.lrecv)
	//DEBUG
	// m.l.Infof("%s = %d(%d-%d), %d(%d-%d)", dt, s-ls, s, ls, r-lr, r, lr)
	//scale to per second V=D/T
	sps := int64(float64(s-ls) / float64(dt) * float64(time.Second))
	rps := int64(float64(r-lr) / float64(dt) * float64(time.Second))
	if last > 0 && (sps != 0 || rps != 0) {
		m.l.Debugf("write %s/s read %s/s", sizestr.ToString(sps), sizestr.ToString(rps))
	}
	//record last printed
	atomic.StoreInt64(&m.lsent, s)
	atomic.StoreInt64(&m.lrecv, r)
	//done
	atomic.StoreInt64(&m.last, curr)
	atomic.StoreUint32(&m.printing, 0)
}

//TeeReader inserts Meter into the read path
//if the linked logger is in debug mode,
//otherwise this is a no-op
func (m *Meter) TeeReader(r io.Reader) io.Reader {
	if m.l.IsDebug() {
		return &meterReader{m, r}
	}
	return r
}

type meterReader struct {
	*Meter
	inner io.Reader
}

func (m *meterReader) Read(p []byte) (n int, err error) {
	n, err = m.inner.Read(p)
	atomic.AddInt64(&m.recv, int64(n))
	m.Meter.print()
	return
}

//TeeWriter inserts Meter into the write path
//if the linked logger is in debug mode,
//otherwise this is a no-op
func (m *Meter) TeeWriter(w io.Writer) io.Writer {
	if m.l.IsDebug() {
		return &meterWriter{m, w}
	}
	return w
}

type meterWriter struct {
	*Meter
	inner io.Writer
}

func (m *meterWriter) Write(p []byte) (n int, err error) {
	n, err = m.inner.Write(p)
	atomic.AddInt64(&m.sent, int64(n))
	m.Meter.print()
	return
}

//MeterConn inserts Meter into the connection path
//if the linked logger is in debug mode,
//otherwise this is a no-op
func MeterConn(l *cio.Logger, conn net.Conn) net.Conn {
	m := NewMeter(l)
	return &meterConn{
		mread:  m.TeeReader(conn),
		mwrite: m.TeeWriter(conn),
		Conn:   conn,
	}
}

type meterConn struct {
	mread  io.Reader
	mwrite io.Writer
	net.Conn
}

func (m *meterConn) Read(p []byte) (n int, err error) {
	return m.mread.Read(p)
}

func (m *meterConn) Write(p []byte) (n int, err error) {
	return m.mwrite.Write(p)
}

//MeterRWC inserts Meter into the RWC path
//if the linked logger is in debug mode,
//otherwise this is a no-op
func MeterRWC(l *cio.Logger, rwc io.ReadWriteCloser) io.ReadWriteCloser {
	m := NewMeter(l)
	return &struct {
		io.Reader
		io.Writer
		io.Closer
	}{
		Reader: m.TeeReader(rwc),
		Writer: m.TeeWriter(rwc),
		Closer: rwc,
	}
}
