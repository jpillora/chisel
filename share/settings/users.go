package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jpillora/chisel/share/cio"
)

type Users struct {
	sync.RWMutex
	inner map[string]*User
}

func NewUsers() *Users {
	return &Users{inner: map[string]*User{}}
}

// Len returns the numbers of users
func (u *Users) Len() int {
	u.RLock()
	l := len(u.inner)
	u.RUnlock()
	return l
}

// Get user from the index by key
func (u *Users) Get(key string) (*User, bool) {
	u.RLock()
	user, found := u.inner[key]
	u.RUnlock()
	return user, found
}

// Set a users into the list by specific key
func (u *Users) Set(key string, user *User) {
	u.Lock()
	u.inner[key] = user
	u.Unlock()
}

// Del ete a users from the list
func (u *Users) Del(key string) {
	u.Lock()
	delete(u.inner, key)
	u.Unlock()
}

// AddUser adds a users to the set
func (u *Users) AddUser(user *User) {
	u.Set(user.Name, user)
}

// Reset all users to the given set,
// Use nil to remove all.
func (u *Users) Reset(users []*User) {
	m := map[string]*User{}
	for _, u := range users {
		m[u.Name] = u
	}
	u.Lock()
	u.inner = m
	u.Unlock()
}

// UserIndex is a reloadable user source
type UserIndex struct {
	*cio.Logger
	*Users
	configFile string
	pinned     []*User
}

// PinUser adds a user which survives configuration file
// reloads (e.g. the --auth user). Pin before LoadUsers.
func (u *UserIndex) PinUser(user *User) {
	u.pinned = append(u.pinned, user)
	u.AddUser(user)
}

// NewUserIndex creates a source for users
func NewUserIndex(logger *cio.Logger) *UserIndex {
	return &UserIndex{
		Logger: logger.Fork("users"),
		Users:  NewUsers(),
	}
}

// LoadUsers is responsible for loading users from a file
func (u *UserIndex) LoadUsers(configFile string) error {
	u.configFile = configFile
	u.Infof("Loading configuration file %s", configFile)
	if err := u.loadUserIndex(); err != nil {
		return err
	}
	if err := u.addWatchEvents(); err != nil {
		return err
	}
	return nil
}

// addWatchEvents is responsible for watching for updates to the file and reloading
func (u *UserIndex) addWatchEvents() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	configPath, err := filepath.Abs(u.configFile)
	if err != nil {
		watcher.Close()
		return err
	}
	//watch the parent directory instead of the file itself, so the watch
	//survives editors and orchestrators which replace the file rather
	//than write to it (vim tmp+rename, truncate+write, kubernetes
	//configmap symlink swaps)
	if err := watcher.Add(filepath.Dir(configPath)); err != nil {
		watcher.Close()
		return err
	}
	//track the resolved path to catch symlink retargets
	realPath, _ := filepath.EvalSymlinks(configPath)
	go func() {
		//collapse event bursts (truncate+write, remove+create) into a
		//single reload once events settle, so half-written files are
		//not loaded
		const debounce = 100 * time.Millisecond
		timer := time.NewTimer(debounce)
		timer.Stop()
		//fsnotify does not reliably deliver directory events for
		//kubelet-style symlink swaps on every platform (notably
		//macOS/kqueue), so reconcile the resolved path periodically
		//as a fallback
		reconcile := time.NewTicker(time.Second)
		defer reconcile.Stop()
		for {
			select {
			case e, ok := <-watcher.Events:
				if !ok {
					return
				}
				if u.watchHit(e, configPath, &realPath) {
					timer.Reset(debounce)
				}
			case <-timer.C:
				if err := u.loadUserIndex(); err != nil {
					u.Infof("Failed to reload the users configuration: %s", err)
				} else {
					u.Debugf("Users configuration successfully reloaded from: %s", u.configFile)
				}
			case <-reconcile.C:
				//catch symlink retargets that arrived without an event
				if current, err := filepath.EvalSymlinks(configPath); err == nil && current != realPath {
					realPath = current
					timer.Reset(debounce)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				u.Infof("Error watching the users configuration: %s", err)
			}
		}
	}()
	return nil
}

// watchHit reports whether the event affects the configured file, either
// directly or via a symlink retarget (kubernetes configmaps swap a
// symlinked directory rather than writing to the watched file).
func (u *UserIndex) watchHit(e fsnotify.Event, configPath string, realPath *string) bool {
	if e.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
		return false //ignore chmod
	}
	if eventPath, err := filepath.Abs(e.Name); err == nil && samePath(eventPath, configPath) {
		return true
	}
	if current, err := filepath.EvalSymlinks(configPath); err == nil && current != *realPath {
		*realPath = current
		return true
	}
	return false
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// loadUserIndex is responsible for loading the users configuration
func (u *UserIndex) loadUserIndex() error {
	if u.configFile == "" {
		return errors.New("configuration file not set")
	}
	b, err := os.ReadFile(u.configFile)
	if err != nil {
		return fmt.Errorf("Failed to read auth file: %s, error: %s", u.configFile, err)
	}
	var raw map[string][]string
	if err := json.Unmarshal(b, &raw); err != nil {
		return errors.New("Invalid JSON: " + err.Error())
	}
	users := []*User{}
	for auth, remotes := range raw {
		user := &User{}
		user.Name, user.Pass = ParseAuth(auth)
		if user.Name == "" {
			return errors.New("Invalid user:pass string")
		}
		for _, r := range remotes {
			if r == "" || r == "*" {
				user.Addrs = append(user.Addrs, UserAllowAll)
			} else {
				re, err := regexp.Compile(r)
				if err != nil {
					return errors.New("Invalid address regex")
				}
				if !strings.HasPrefix(r, "^") || !strings.HasSuffix(r, "$") {
					u.Infof("authfile: pattern %q (user %s) is unanchored and "+
						"may match unintended addresses; anchor it with ^ and $", r, user.Name)
				}
				user.Addrs = append(user.Addrs, re)
			}
		}
		users = append(users, user)
	}
	//swap, keeping pinned users (pinned last: they win name clashes)
	u.Reset(append(users, u.pinned...))
	return nil
}
