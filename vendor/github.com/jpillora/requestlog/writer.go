package requestlog

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/jpillora/sizestr"
)

func monitorWriter(w http.ResponseWriter, r *http.Request, opts *Options) *monitorableWriter {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	if ip == "127.0.0.1" || ip == "::1" {
		ip = ""
	}
	return &monitorableWriter{
		opts:   opts,
		t0:     time.Now(),
		w:      w,
		r:      r,
		method: r.Method,
		path:   r.URL.Path,
		ip:     ip,
	}
}

//monitorable ResponseWriter
type monitorableWriter struct {
	opts *Options
	t0   time.Time
	//handler
	w http.ResponseWriter
	r *http.Request
	//stats
	method, path, ip string
	Code             int
	Size             int64
}

func (m *monitorableWriter) Header() http.Header {
	return m.w.Header()
}

func (m *monitorableWriter) Write(p []byte) (int, error) {
	m.Size += int64(len(p))
	return m.w.Write(p)
}

func (m *monitorableWriter) WriteHeader(c int) {
	m.Code = c
	m.w.WriteHeader(c)
}

func (m *monitorableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := m.w.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijacking not supported")
	}
	return hj.Hijack()
}

func (m *monitorableWriter) Flush() {
	m.w.(http.Flusher).Flush()
}

func (m *monitorableWriter) CloseNotify() <-chan bool {
	return m.w.(http.CloseNotifier).CloseNotify()
}

var integerRegexp = regexp.MustCompile(`\.\d+`)

//replace ResponseWriter with a monitorable one, return logger
func (m *monitorableWriter) Log() {
	duration := time.Now().Sub(m.t0)
	if m.Code == 0 {
		m.Code = 200
	}
	if m.opts.Filter != nil && !m.opts.Filter(m.r, m.Code, duration, m.Size) {
		return //skip
	}
	cc := m.colorCode()
	size := ""
	if m.Size > 0 {
		size = sizestr.ToString(m.Size)
	}
	buff := bytes.Buffer{}
	m.opts.formatTmpl.Execute(&buff, &struct {
		*Colors
		Timestamp, Method, Path, CodeColor string
		Code                               int
		Duration, Size, IP                 string
	}{
		m.opts.Colors,
		m.t0.Format(m.opts.TimeFormat), m.method, m.path, cc,
		m.Code,
		fmtDuration(duration), size, m.ip,
	})
	//fmt is threadsafe :)
	fmt.Fprint(m.opts.Writer, buff.String())
}

func (m *monitorableWriter) colorCode() string {
	switch m.Code / 100 {
	case 2:
		return m.opts.Colors.Green
	case 3:
		return m.opts.Colors.Cyan
	case 4:
		return m.opts.Colors.Yellow
	case 5:
		return m.opts.Colors.Red
	}
	return m.opts.Colors.Grey
}
