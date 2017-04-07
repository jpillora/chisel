package chshare

import (
	"encoding/json"
	"errors"
	"io/ioutil"
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

type Users map[string]*User

func ParseUsers(authfile string) (Users, error) {

	b, err := ioutil.ReadFile(authfile)
	if err != nil {
		return nil, errors.New("Failed to read auth file")
	}

	var raw map[string][]string
	err = json.Unmarshal(b, &raw)
	if err != nil {
		return nil, errors.New("Invalid JSON: " + err.Error())
	}

	users := Users{}
	for auth, remotes := range raw {
		u := &User{}
		u.Name, u.Pass = ParseAuth(auth)
		if u.Name == "" {
			return nil, errors.New("Invalid user:pass string")
		}
		for _, r := range remotes {
			if r == "" || r == "*" {
				u.Addrs = append(u.Addrs, UserAllowAll)
			} else {
				re, err := regexp.Compile(r)
				if err != nil {
					return nil, errors.New("Invalid address regex")
				}
				u.Addrs = append(u.Addrs, re)
			}

		}
		users[u.Name] = u
	}
	return users, nil
}
