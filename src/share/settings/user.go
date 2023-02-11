package settings

import (
	"regexp"
	"strings"
)

var UserAllowAll = regexp.MustCompile("")

func ParseAuth(auth string) (string, string) {
	if strings.Contains(auth, ":") {
		pair := strings.SplitN(auth, ":", 2)
		return pair[0], pair[1]
	}
	return "", ""
}

type User struct {
	Name  string
	Pass  string
	Addrs []*regexp.Regexp
}

func (u *User) HasAccess(addr string) bool {
	m := false
	for _, r := range u.Addrs {
		if r.MatchString(addr) {
			m = true
			break
		}
	}
	return m
}
