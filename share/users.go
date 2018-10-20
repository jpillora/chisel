package chshare

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type Users map[string]*User

// UserIndex is a reloadable user source
type UserIndex struct {
	*Logger
	sync.RWMutex
	configFile string
	users      Users
}

// NewUserIndex creates a source for users
func NewUserIndex(logger *Logger) *UserIndex {
	return &UserIndex{
		Logger: logger.Fork("users"),
		users:  make(Users, 0),
	}
}

// LoadUsers is responsible for loading users from a file
func (u *UserIndex) LoadUsers(configFile string) error {
	u.configFile = configFile
	u.Infof("Loading the configuraion from: %s", configFile)
	if err := u.loadUserIndex(); err != nil {
		return err
	}
	if err := u.addWatchEvents(); err != nil {
		return err
	}
	return nil
}

// Len returns the numbers of users
func (u *UserIndex) Len() int {
	u.RLock()
	l := len(u.users)
	u.RUnlock()
	return l
}

// GetUser retrieves a user from the index
func (u *UserIndex) GetUser(username string) (*User, bool) {
	u.RLock()
	user, found := u.users[username]
	u.RUnlock()
	return user, found
}

// AddUser adds a users to the list
func (u *UserIndex) AddUser(user *User) {
	u.Lock()
	u.users[user.Name] = user
	u.Unlock()
}

// watchEvents is responsible for watching for updates to the file and reloading
func (u *UserIndex) addWatchEvents() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	configDir := filepath.Dir(u.configFile)
	if err := watcher.Add(configDir); err != nil {
		return err
	}
	go func() {
		for e := range watcher.Events {
			if e.Name != u.configFile {
				continue
			}
			if e.Op&fsnotify.Write != fsnotify.Write {
				continue
			}
			if err := u.loadUserIndex(); err != nil {
				u.Infof("Failed to reload the users configuration: %s", err)
			} else {
				u.Debugf("Users configuration successfully reloaded from: %s", u.configFile)
			}
		}
	}()
	return nil
}

// loadUserIndex is responsible for loading the users configuration
func (u *UserIndex) loadUserIndex() error {
	if u.configFile == "" {
		return errors.New("configuration file not set")
	}
	b, err := ioutil.ReadFile(u.configFile)
	if err != nil {
		return fmt.Errorf("Failed to read auth file: %s, error: %s", u.configFile, err)
	}
	var raw map[string][]string
	if err := json.Unmarshal(b, &raw); err != nil {
		return errors.New("Invalid JSON: " + err.Error())
	}
	users := Users{}
	for auth, remotes := range raw {
		u := &User{}
		u.Name, u.Pass = ParseAuth(auth)
		if u.Name == "" {
			return errors.New("Invalid user:pass string")
		}
		for _, r := range remotes {
			if r == "" || r == "*" {
				u.Addrs = append(u.Addrs, UserAllowAll)
			} else {
				re, err := regexp.Compile(r)
				if err != nil {
					return errors.New("Invalid address regex")
				}
				u.Addrs = append(u.Addrs, re)
			}

		}
		users[u.Name] = u
	}
	u.Lock()
	u.users = users
	u.Unlock()
	return nil
}
