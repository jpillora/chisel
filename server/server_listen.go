package chserver

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"golang.org/x/crypto/acme/autocert"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"path/filepath"
)

//TLSConfig enables configures TLS
type TLSConfig struct {
	Key     string
	Cert    string
	Domains []string
	CA      string
}

func (s *Server) listener(host, port string) (net.Listener, error) {
	hasDomains := len(s.config.TLS.Domains) > 0
	hasKeyCert := s.config.TLS.Key != "" && s.config.TLS.Cert != ""
	if hasDomains && hasKeyCert {
		return nil, errors.New("cannot use key/cert and domains")
	}
	var tlsConf *tls.Config
	if hasDomains {
		tlsConf = s.tlsLetsEncrypt(s.config.TLS.Domains)
	}
	extra := ""
	if hasKeyCert {
		c, err := s.tlsKeyCert(s.config.TLS.Key, s.config.TLS.Cert, s.config.TLS.CA)
		if err != nil {
			return nil, err
		}
		tlsConf = c
		if port != "443" {
			extra = " (WARNING: lets-encrypt must connect to your domains on port 443)"
		}
	}
	//tcp listen
	l, err := net.Listen("tcp", host+":"+port)
	//optionally wrap in tls
	proto := "http"
	if tlsConf != nil {
		proto += "s"
		l = tls.NewListener(l, tlsConf)
	}
	if err == nil {
		s.Infof("Listening on %s://%s:%s%s", proto, host, port, extra)
	}
	return l, nil
}

func (s *Server) tlsLetsEncrypt(domains []string) *tls.Config {
	//prepare cert manager
	m := &autocert.Manager{
		Prompt: func(tosURL string) bool {
			s.Infof("Accepting LetsEncrypt TOS and fetching certificate...")
			return true
		},
		Email:      os.Getenv("CHISEL_LE_EMAIL"),
		HostPolicy: autocert.HostWhitelist(domains...),
	}
	//configure file cache
	c := os.Getenv("CHISEL_LE_CACHE")
	if c == "" {
		h := os.Getenv("HOME")
		if h == "" {
			if u, err := user.Current(); err == nil {
				h = u.HomeDir
			}
		}
		c = filepath.Join(h, ".cache", "chisel")
	}
	if c != "-" {
		s.Infof("LetsEncrypt cache directory %s", c)
		m.Cache = autocert.DirCache(c)
	}
	//return lets-encrypt tls config
	return m.TLSConfig()
}

func (s *Server) tlsKeyCert(key, cert string, clientCaPath string) (*tls.Config, error) {
	c, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	//file based tls config using tls defaults
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{c},
	}
	//mTLS requires Client CAs
	if clientCaPath != "" {
		fileInfo, err := os.Stat(clientCaPath)
		if err != nil {
			return nil, err
		}
		clientCAPool := x509.NewCertPool()
		if fileInfo.IsDir() {
			//this is a directory holding CA bundle files
			files, err := ioutil.ReadDir(clientCaPath)
			if err != nil {
				return nil, err
			}
			s.Infof("Looking for client CA files from %s", clientCaPath)
			//add all cert files from path
			for _, file := range files {
				f := file.Name()
				if err := addCABundle(filepath.Join(clientCaPath, f), clientCAPool); err != nil {
					return nil, err
				}
			}
		} else {
			//this is a CA bundle file
			if err := addCABundle(clientCaPath, clientCAPool); err != nil {
				return nil, err
			}
		}
		//set client CAs and enable cert verification
		tlsCfg.ClientCAs = clientCAPool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return tlsCfg, nil
}

func addCABundle(clientCaPath string, clientCAPool *x509.CertPool) error {
	if content, err := ioutil.ReadFile(clientCaPath); err != nil {
		return nil
	} else if !clientCAPool.AppendCertsFromPEM(content) {
		return errors.New("Fail to load certificates from : " + clientCaPath)
	} else {
		return err
	}
}
