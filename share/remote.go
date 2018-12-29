package chshare

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// short-hand conversions
//   3000 ->
//     local  127.0.0.1:3000
//     remote 127.0.0.1:3000
//   foobar.com:3000 ->
//     local  127.0.0.1:3000
//     remote foobar.com:3000
//   3000:google.com:80 ->
//     local  127.0.0.1:3000
//     remote google.com:80
//   192.168.0.1:3000:google.com:80 ->
//     local  192.168.0.1:3000
//     remote google.com:80

type Remote struct {
	LocalHost, LocalPort, RemoteHost, RemotePort string
	Socks, Reverse                               bool
}

const revPrefix = "R:"

func DecodeRemote(s string) (*Remote, error) {
	reverse := false
	if strings.HasPrefix(s, revPrefix) {
		s = strings.TrimPrefix(s, revPrefix)
		reverse = true
	}
	parts := strings.Split(s, ":")
	if len(parts) <= 0 || len(parts) >= 5 {
		return nil, errors.New("Invalid remote")
	}
	r := &Remote{Reverse: reverse}
	for i := len(parts) - 1; i >= 0; i-- {
		p := parts[i]
		//last part "socks"?
		if i == len(parts)-1 && p == "socks" {
			if reverse {
				// TODO allow reverse+socks by having client
				// automatically start local SOCKS5 server
				return nil, errors.New("'socks' incompatible with reverse port forwarding")
			}
			r.Socks = true
			continue
		}
		if isPort(p) {
			if !r.Socks && r.RemotePort == "" {
				r.RemotePort = p
				r.LocalPort = p
			} else {
				r.LocalPort = p
			}
			continue
		}
		if !r.Socks && (r.RemotePort == "" && r.LocalPort == "") {
			return nil, errors.New("Missing ports")
		}
		if !isHost(p) {
			return nil, errors.New("Invalid host")
		}
		if !r.Socks && r.RemoteHost == "" {
			r.RemoteHost = p
		} else {
			r.LocalHost = p
		}
	}
	if r.LocalHost == "" {
		if r.Socks {
			r.LocalHost = "127.0.0.1"
		} else {
			r.LocalHost = "0.0.0.0"
		}
	}
	if r.LocalPort == "" && r.Socks {
		r.LocalPort = "1080"
	}
	if !r.Socks && r.RemoteHost == "" {
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

func isHost(s string) bool {
	_, err := url.Parse(s)
	if err != nil {
		return false
	}
	return true
}

//implement Stringer
func (r *Remote) String() string {
	tag := ""
	if r.Reverse {
		tag = revPrefix
	}
	return tag + r.LocalHost + ":" + r.LocalPort + "=>" + r.Remote()
}

func (r *Remote) Remote() string {
	if r.Socks {
		return "socks"
	}
	return r.RemoteHost + ":" + r.RemotePort
}
