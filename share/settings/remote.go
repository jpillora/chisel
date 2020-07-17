package settings

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// short-hand conversions (see remote_test)
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
//   127.0.0.1:1080:socks
//     local  127.0.0.1:1080
//     remote socks
//   stdio:example.com:22
//     local  stdio
//     remote example.com:22
//   1.1.1.1:53/udp
//     local  127.0.0.1:53/udp
//     remote 1.1.1.1:53/udp

type Remote struct {
	LocalHost, LocalPort, LocalProto    string
	RemoteHost, RemotePort, RemoteProto string
	Socks, Reverse, Stdio               bool
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
	//parse from back to front, to set 'remote' fields first,
	//then to set 'local' fields second (allows the 'remote' side
	//to provide the defaults)
	for i := len(parts) - 1; i >= 0; i-- {
		p := parts[i]
		//remote portion is socks?
		if i == len(parts)-1 && p == "socks" {
			r.Socks = true
			continue
		}
		//local portion is stdio?
		if i == 0 && p == "stdio" {
			r.Stdio = true
			continue
		}
		p, proto := L4Proto(p)
		if proto != "" {
			if r.RemotePort == "" {
				r.RemoteProto = proto
			} else if r.LocalProto == "" {
				r.LocalProto = proto
			}
		}
		if isPort(p) {
			if !r.Socks && r.RemotePort == "" {
				r.RemotePort = p
			}
			r.LocalPort = p
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
	//remote string parsed, apply defaults...
	if r.Socks {
		//socks defaults
		if r.LocalHost == "" {
			r.LocalHost = "127.0.0.1"
		}
		if r.LocalPort == "" {
			r.LocalPort = "1080"
		}
	} else {
		//non-socks defaults
		if r.LocalHost == "" {
			r.LocalHost = "0.0.0.0"
		}
		if r.RemoteHost == "" {
			r.RemoteHost = "127.0.0.1"
		}
	}
	if r.RemoteProto == "" {
		r.RemoteProto = "tcp"
	}
	if r.LocalProto == "" {
		r.LocalProto = r.RemoteProto
	}
	if r.LocalProto != r.RemoteProto {
		return nil, errors.New("currently, local and remote protocols must match")
	}
	if r.Stdio && r.Reverse {
		return nil, errors.New("stdio cannot be reversed")
	}
	return r, nil
}

func isPort(s string) bool {
	n, err := strconv.Atoi(s)
	if err != nil {
		return false
	}
	if n <= 0 || n > 65535 {
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

var l4Proto = regexp.MustCompile(`(?i)\/(tcp|udp)$`)

//L4Proto extacts the layer-4 protocol from the given string
func L4Proto(s string) (head, proto string) {
	if l4Proto.MatchString(s) {
		l := len(s)
		return strings.ToLower(s[:l-4]), s[l-3:]
	}
	return s, ""
}

//implement Stringer
func (r Remote) String() string {
	tag := ""
	if r.Reverse {
		tag = revPrefix
	}
	return tag + r.Local() + "=>" + r.Remote()
}

//Encode remote to a string
func (r Remote) Encode() string {
	if r.LocalPort == "" {
		r.LocalPort = r.RemotePort
	}
	local := r.Local()
	remote := r.Remote()
	if r.RemoteProto == "udp" {
		remote += "/udp"
	}
	if r.Reverse {
		return "R:" + local + ":" + remote
	}
	return local + ":" + remote
}

func (r Remote) Local() string {
	if r.Stdio {
		return "stdio"
	}
	if r.LocalHost == "" {
		r.LocalHost = "127.0.0.1"
	}
	return r.LocalHost + ":" + r.LocalPort
}

func (r Remote) Remote() string {
	if r.Socks {
		return "socks"
	}
	if r.RemoteHost == "" {
		r.RemoteHost = "0.0.0.0"
	}
	return r.RemoteHost + ":" + r.RemotePort
}

func (r Remote) Access() string {
	if r.Reverse {
		return "R:" + r.LocalHost + ":" + r.LocalPort
	}
	return r.RemoteHost + ":" + r.RemotePort
}

type Remotes []*Remote

//Filter out forward reversed/non-reversed remotes
func (rs Remotes) Reversed(reverse bool) Remotes {
	subset := Remotes{}
	for _, r := range rs {
		match := r.Reverse == reverse
		if match {
			subset = append(subset, r)
		}
	}
	return subset
}

//Encode back into strings
func (rs Remotes) Encode() []string {
	s := make([]string, len(rs))
	for i, r := range rs {
		s[i] = r.Encode()
	}
	return s
}
