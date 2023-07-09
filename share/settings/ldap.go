package settings

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/go-ldap/ldap/v3"
)

// LDAPConfig enables LDAP auth
type LDAPConfig struct {
	BindDN       string `json:"bindDN"`
	BindPassword string `json:"bindPassword"`
	URL          string `json:"url"`
	BaseDN       string `json:"baseDN"`
	Filter       string `json:"filter"`
	IDMapTo      string `json:"idMapTo"`
	CA           string `json:"ca"`
	Insecure     bool   `json:"insecure"`
}

// parse the LDAP config file
func LDAPParseConfig(path string) (*LDAPConfig, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("LDAP config file error")
	}
	config := &LDAPConfig{}
	err = json.Unmarshal([]byte(file), config)
	if err != nil {
		return nil, fmt.Errorf("error occured during unmarshaling ldap config file")
	}
	return config, nil
}

// authenticate a user using ldap credentials
func LDAPAuthUser(user *User, password []byte, config *LDAPConfig) error {
	log.Printf("User %s to be authenticated in LDAP", user.Name)
	l, err := connectTLS(config)
	if err != nil {
		log.Printf("Error occured during TLS connection to %s", config.URL)
		return fmt.Errorf("error occured during TLS connection to %s", config.URL)
	}
	defer l.Close()
	// Normal Bind and Search
	result, err := bindAndSearch(l, config, user)
	if err != nil {
		log.Printf("User %s not found in LDAP", user.Name)
		return fmt.Errorf("User %s not found in LDAP", user.Name)
	}
	userdn := result.Entries[0].DN
	log.Printf("DN:%s", userdn)
	if len(result.Entries) != 1 {
		log.Printf("too many entries returned for user %s", user.Name)
		return fmt.Errorf("too many entries returned")
	}
	// Bind as the user to verify their password
	err = l.Bind(userdn, string(password[:]))
	if err != nil {
		return fmt.Errorf("bad password for user %s", user.Name)
	}

	return nil
}

// LDAP Connection with TLS
func connectTLS(ldapconfig *LDAPConfig) (*ldap.Conn, error) {
	var tlsConf *tls.Config

	if ldapconfig.Insecure {
		tlsConf = &tls.Config{InsecureSkipVerify: true}
	}

	if ldapconfig.CA != "" {
		log.Printf("CA file %s", ldapconfig.CA)
		certpool := x509.NewCertPool()
		CAfile, err := os.ReadFile(ldapconfig.CA)
		if err != nil {
			log.Printf("LDAP CA file error")
			return nil, fmt.Errorf("LDAP CA file error")
		}
		certpool.AppendCertsFromPEM([]byte(CAfile))
		tlsConf = &tls.Config{RootCAs: certpool}
		log.Printf("CA file %s loaded", ldapconfig.CA)
	}

	l, err := ldap.DialTLS("tcp", ldapconfig.URL, tlsConf)
	if err != nil {
		log.Printf("TLS error: %s", err)
		return nil, err
	}

	return l, nil
}

// Normal Bind and Search
func bindAndSearch(l *ldap.Conn, config *LDAPConfig, user *User) (*ldap.SearchResult, error) {
	l.Bind(config.BindDN, config.BindPassword)
	var filter string
	if config.Filter != "" {
		filter = fmt.Sprintf("(&(%s)(%s=%s))", config.Filter, config.IDMapTo, user.Name)
	} else {
		filter = fmt.Sprintf("(%s=%s)", config.IDMapTo, user.Name)
	}
	log.Printf("filter %s", filter)
	searchReq := ldap.NewSearchRequest(
		config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		filter,
		[]string{"dn"},
		nil,
	)
	result, err := l.Search(searchReq)
	if err != nil {
		log.Printf("Search Error: %s", err)
		return nil, fmt.Errorf("search Error: %s", err)
	}
	if len(result.Entries) == 0 {
		return nil, fmt.Errorf("couldn't fetch search entries")
	}
	return result, nil
}
