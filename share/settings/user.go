package settings

import (
	"regexp"
	"strings"
	"fmt"
	"syscall"
	"os"
	"io/ioutil"
	"golang.org/x/term"
)

var UserAllowAll = regexp.MustCompile("")

//check for any errors produced
func check(e error) {
	if e != nil {
		os.Exit(1)
	}
}

//trim any carriage returns (\r) or new lines (\n) from string
func TrimAllNewLines(s string) string{
	re := regexp.MustCompile(`\r?\n`)
	return re.ReplaceAllString(s, "")
}

func ParseAuth(auth string) (string, string) {
	if strings.Contains(auth, ":") {
		pair := strings.SplitN(auth, ":", 2)
		return pair[0], pair[1]
	} else if auth == "stdin" {
		//read in username and then password
		var user_in string
		fmt.Print("Username: ")
		fmt.Scanln(&user_in)
		fmt.Print("Password: ")
		bytepw, err := term.ReadPassword(int(syscall.Stdin))
		check(err)
		fmt.Println("")
		pass := string(bytepw)
		return user_in,pass
	} else if strings.Contains(auth, ">") {
		//read file for authorization
		authfile := strings.SplitN(auth, ">", 2)
		byteline,err := ioutil.ReadFile(authfile[1])
		check(err)
		line := string(byteline)
		if strings.Contains(line, ":") {
			pair := strings.SplitN(TrimAllNewLines(line), ":", 2)
			return pair[0], pair[1]
		} else {
			fmt.Println("*** File provided authentication error ***")
			return "", ""
		}
	} else if strings.Contains(auth, "=") {
		//read from environment variable AUTH
		env := os.Getenv(strings.SplitN(auth, "=", 2)[1])
		if strings.Contains(env, ":") {
			pair := strings.SplitN(env, ":", 2)
			return pair[0], pair[1]
		} else {
			fmt.Println("*** Environment variable authentication error ***")
			return "", ""
		}
	}
	fmt.Println("*** String provided authentication error (i.e. \"user:password\" ***")
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
