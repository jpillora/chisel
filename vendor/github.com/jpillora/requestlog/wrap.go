package requestlog

import (
	"io"
	"net/http"
	"os"
	"regexp"
	"text/template"
	"time"

	"github.com/andrew-d/go-termutil"
	"github.com/jpillora/ansi"
	"github.com/jpillora/sizestr"
)

func init() {
	sizestr.UpperCase()
}

type Colors struct {
	Grey, Green, Cyan, Yellow, Red, Reset string
}

var basicColors = &Colors{string(ansi.ResetBytes), string(ansi.GreenBytes), string(ansi.CyanBytes), string(ansi.YellowBytes), string(ansi.YellowBytes), string(ansi.ResetBytes)}
var noColors = &Colors{} //no colors

type Options struct {
	Writer     io.Writer
	TimeFormat string
	Format     string
	formatTmpl *template.Template
	Colors     *Colors
	Filter     func(r *http.Request, code int, duration time.Duration, size int64) bool
	//TrustProxy will log X-Forwarded-For/X-Real-Ip instead of the IP source
	TrustProxy bool
}

var DefaultOptions = Options{
	Writer:     os.Stdout,
	TimeFormat: "2006/01/02 15:04:05.000",
	Format: `{{ if .Timestamp }}{{ .Timestamp }} {{end}}` +
		`{{ .Method }} {{ .Path }} {{ .CodeColor }}{{ .Code }}{{ .Reset }} ` +
		`{{ .Duration }}{{ if .Size }} {{ .Size }}{{end}}` +
		`{{ if .IP }} ({{ .IP }}){{end}}` + "\n",
	Colors:     defaultColors(),
	TrustProxy: false,
}

func defaultColors() *Colors {
	if termutil.Isatty(os.Stdout.Fd()) {
		return basicColors
	}
	return noColors
}

func Wrap(next http.Handler) http.Handler {
	return WrapWith(next, Options{})
}

func WrapWith(next http.Handler, opts Options) http.Handler {
	if opts.Writer == nil {
		opts.Writer = DefaultOptions.Writer
	}
	if opts.TimeFormat == "" {
		opts.TimeFormat = DefaultOptions.TimeFormat
	}
	if opts.Format == "" {
		opts.Format = DefaultOptions.Format
	}
	if opts.Colors == nil {
		opts.Colors = DefaultOptions.Colors
	}
	var err error
	opts.formatTmpl, err = template.New("format").Parse(opts.Format)
	if err != nil {
		panic(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := monitorWriter(w, r, &opts)
		next.ServeHTTP(m, r)
		m.Log()
	})
}

var fmtDurationRe = regexp.MustCompile(`\.\d+`)

func fmtDuration(t time.Duration) string {
	return fmtDurationRe.ReplaceAllString(t.String(), "")
}
