package chshare

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// UserSource is a reloadable user source
type UserSource struct {
	sync.RWMutex
	configFile string
	logger     *Logger
	users      Users
}

// NewUserSource
func NewUserSource(logger *Logger) *UserSource {
	return &UserSource{
		logger: logger,
		users:  make(Users, 0),
	}
}

// LoadUsers is responsible for loading users from a file
func (u *UserSource) LoadUsers(filename string, reloadable bool) error {
	u.configFile = filename

	u.logger.Infof("Loading the configuraion from: %s", filename)
	if err := u.loadUserSource(); err != nil {
		return err
	}

	if reloadable {
		u.logger.Infof("Enabling reloading of configuration")
		if err := u.addWatchEvents(); err != nil {
			return err
		}
	}

	return nil
}

// Size returns the numbers of users
func (u *UserSource) Size() int {
	u.RLock()
	defer u.RUnlock()

	return len(u.users)
}

// HasUser is responsible for checking the user exists
func (u *UserSource) HasUser(username string) (*User, bool) {
	u.RLock()
	defer u.RUnlock()

	if u, found := u.users[username]; found {
		return u, true
	}

	return nil, false
}

// AddUser adds a users to the list
func (u *UserSource) AddUser(user *User) {
	u.Lock()
	defer u.Unlock()

	u.users[user.Name] = user
}

// watchEvents is responsible for watching for updates to the file and reloading
func (u *UserSource) addWatchEvents() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(filepath.Dir(u.configFile)); err != nil {
		return err
	}

	go func() {
		for e := range watcher.Events {
			if e.Name != u.configFile {
				continue
			}

			if e.Op&fsnotify.Write == fsnotify.Write {
				u.logger.Debugf("User configuration has changed: %s", u.configFile)

				if err := u.loadUserSource(); err != nil {
					u.logger.Errorf("Failed to reload the users configuration: %s", err)
					continue
				}
				u.logger.Debugf("Users configuration successfully reloaded from: %s", u.configFile)
			}
		}
	}()

	return nil
}

// loadUserSource is responsible for loading the users configuration
func (u *UserSource) loadUserSource() error {
	if u.configFile == "" {
		return errors.New("configuration file not set")
	}

	users, err := ParseUsers(u.configFile)
	if err != nil {
		return err
	}

	u.Lock()
	defer u.Unlock()
	u.users = users

	return nil
}
