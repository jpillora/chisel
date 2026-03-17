package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/jpillora/chisel/share/cio"
)

// URLUserIndex authenticates users against an external HTTP endpoint.
// On every login attempt it POSTs {"username": "...", "password": "..."}
// to the configured URL. A 200 response must contain a JSON array of address
// regexes (matching the values format of --authfile) to grant access; any
// other status code denies access.
type URLUserIndex struct {
	*cio.Logger
	url        string
	httpClient *http.Client
}

// NewURLUserIndex creates a URLUserIndex that will POST credentials to authURL.
func NewURLUserIndex(authURL string, logger *cio.Logger) *URLUserIndex {
	return &URLUserIndex{
		Logger:     logger.Fork("url-users"),
		url:        authURL,
		httpClient: &http.Client{},
	}
}

// GetUser authenticates a user against the external URL and returns the
// resolved User (with compiled address regexes) on success, or an error on
// failure. An empty string or "*" in the address list grants full access
// (equivalent to UserAllowAll).
func (u *URLUserIndex) GetUser(name, pass string) (*User, error) {
	body, err := json.Marshal(struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{Username: name, Password: pass})
	if err != nil {
		return nil, err
	}
	resp, err := u.httpClient.Post(u.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth denied (status %d)", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var addrStrs []string
	if err := json.Unmarshal(raw, &addrStrs); err != nil {
		return nil, fmt.Errorf("invalid JSON in auth response: %w", err)
	}
	addrs := make([]*regexp.Regexp, 0, len(addrStrs))
	for _, s := range addrStrs {
		if s == "" || s == "*" {
			addrs = append(addrs, UserAllowAll)
		} else {
			re, err := regexp.Compile(s)
			if err != nil {
				return nil, fmt.Errorf("invalid address regex %q: %w", s, err)
			}
			addrs = append(addrs, re)
		}
	}
	return &User{Name: name, Addrs: addrs}, nil
}
