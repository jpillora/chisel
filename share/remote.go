package chshare

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// short-hand conversions
//   foobar.com:3000 ->
//		local 127.0.0.1:3000
//		remote foobar.com:3000
//   3000:google.com:80 ->
//		local 127.0.0.1:3000
//		remote google.com:80
//   192.168.0.1:3000:google.com:80 ->
//		local 192.168.0.1:3000
//		remote google.com:80

type Remote struct {
	LocalHost, LocalPort, RemoteHost, RemotePort string
}

func DecodeRemote(s string) (*Remote, error) {
	parts := strings.Split(s, ":")

	if len(parts) <= 0 || len(parts) >= 5 {
		return nil, errors.New("Invalid remote")
	}

	r := &Remote{}

	//TODO fix up hacky decode
	for i := len(parts) - 1; i >= 0; i-- {
		p := parts[i]

		if isPort(p) {
			if r.RemotePort == "" {
				r.RemotePort = p
				r.LocalPort = p
			} else {
				r.LocalPort = p
			}
			continue
		}

		if r.RemotePort == "" && r.LocalPort == "" {
			return nil, errors.New("Missing ports")
		}

		if !isHost(p) {
			return nil, errors.New("Invalid host")
		}

		if r.RemoteHost == "" {
			r.RemoteHost = p
		} else {
			r.LocalHost = p
		}
	}
	if r.LocalHost == "" {
		r.LocalHost = "0.0.0.0"
	}
	if r.RemoteHost == "" {
		r.RemoteHost = "0.0.0.0"
	}
	return r, nil
}

var isPortRegExp = regexp.MustCompile(`^\d+$`)

func isPort(s string) bool {
	if !isPortRegExp.MatchString(s) {
		return false
	}
	return true
}

var isHTTP = regexp.MustCompile(`^http?:\/\/`)

func isHost(s string) bool {
	_, err := url.Parse(s)
	if err != nil {
		return false
	}
	return true
}

//implement Stringer
func (r *Remote) String() string {
	return r.LocalHost + ":" + r.LocalPort + " => " +
		r.RemoteHost + ":" + r.RemotePort
}
