package settings

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/go-ldap/ldap/v3"
)

//LdapConfig enables LDAP auth
type LdapConfig struct {
	BindDN     			string `json:"BindDN"`
	BindPassword		string `json:"BindPassword"`
	Url							string `json:"Url"`
	BaseDN       		string `json:"BaseDN"`
	Filter       		string `json:"Filter"`
	IDMapTo					string `json:"IDMapTo"`
	CA				      string `json:"CA"`
	Insecure				bool	 `json:"Insecure"`
}

// parse the Ldap config file
func ParseConfigFile(Configfile string) (LdapConfig, error) {
	var ldapConfig LdapConfig
	file, err := ioutil.ReadFile(Configfile)
	if err != nil {
		return ldapConfig, fmt.Errorf("Ldap config file error")
	}
	err = json.Unmarshal([]byte(file), &ldapConfig)
	if err != nil {
		return ldapConfig, fmt.Errorf("Error occured during unmarshaling ldap config file")
	}
	return ldapConfig, nil
}

// authenticate a user using ldap credentials
func LdapAuthUser(user *User,password []byte,ldapconfig LdapConfig) error {
	log.Printf("User %s to be authenticated in LDAP",user.Name)
  l, err := ConnectTLS(ldapconfig)
  if err != nil {
		log.Printf("Error occured during TLS connection to %s",ldapconfig.Url)
    return fmt.Errorf("Error occured during TLS connection to %s",ldapconfig.Url)
  }
  defer l.Close()
  // Normal Bind and Search
  result, err := BindAndSearch(l,ldapconfig,user)
  if err != nil {
		log.Printf("User %s not found in LDAP",user.Name)
    return fmt.Errorf("User %s not found in LDAP",user.Name)
  }
	userdn := result.Entries[0].DN
	log.Printf("DN:%s",userdn)

	if len(result.Entries) != 1 {
		log.Printf("too many entries returned for user %s",user.Name)
		return fmt.Errorf("too many entries returned")
	}
// Bind as the user to verify their password
	err = l.Bind(userdn,string(password[:]))
	if err != nil {
		return fmt.Errorf("Bad password for user %s",user.Name)
	}

	return nil
}

// Ldap Connection with TLS
func ConnectTLS(ldapconfig LdapConfig) (*ldap.Conn, error) {
	var tlsConf *tls.Config

	if ldapconfig.Insecure {
		tlsConf = &tls.Config{InsecureSkipVerify: true}
	}

	if ldapconfig.CA != "" {
		log.Printf("CA file %s",ldapconfig.CA)
		certpool := x509.NewCertPool()
		CAfile, err := ioutil.ReadFile(ldapconfig.CA)
		if err != nil {
			log.Printf("Ldap CA file error")
			return nil, fmt.Errorf("Ldap CA file error")
		}
		certpool.AppendCertsFromPEM([]byte(CAfile))
		tlsConf = &tls.Config{RootCAs: certpool}
		log.Printf("CA file %s loaded", ldapconfig.CA)
	}

	l, err := ldap.DialTLS("tcp", ldapconfig.Url, tlsConf)
  if err != nil {
		log.Printf("TLS error: %s", err)
    return nil, err
  }

  return l, nil
}

// Normal Bind and Search
func BindAndSearch(l *ldap.Conn,ldapconfig LdapConfig,user *User) (*ldap.SearchResult, error) {
  var filter string
	l.Bind(ldapconfig.BindDN, ldapconfig.BindPassword)
	if ldapconfig.Filter != "" {
		filter = fmt.Sprintf("(&(%s)(%s=%s))",ldapconfig.Filter,ldapconfig.IDMapTo,user.Name)
	} else {
		filter = fmt.Sprintf("(%s=%s)",ldapconfig.IDMapTo,user.Name)
	}
	log.Printf("filter %s",filter)
  searchReq := ldap.NewSearchRequest(
      ldapconfig.BaseDN,
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
    return nil, fmt.Errorf("Search Error: %s", err)
  }

  if len(result.Entries) > 0 {
    return result, nil
  } else {
    return nil, fmt.Errorf("Couldn't fetch search entries")
  }
}
