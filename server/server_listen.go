package chserver

import (
	"crypto/tls"
	"net"
)

// TLSConfig enables configures TLS
type TLSConfig struct {
	Key     string
	Cert    string
	Domains []string
	CA      string
}

func (s *Server) listener(host, port string) (net.Listener, error) {
	hasKeyCert := s.config.TLS.Key != "" && s.config.TLS.Cert != ""
	var tlsConf *tls.Config
	extra := ""
	if hasKeyCert {
		c, err := s.tlsKeyCert(s.config.TLS.Key, s.config.TLS.Cert)
		if err != nil {
			return nil, err
		}
		tlsConf = c
		if port != "443" {
			extra = " (WARNING: LetsEncrypt will attempt to connect to your domain on port 443)"
		}
	}
	//tcp listen
	l, err := net.Listen("tcp", host+":"+port)
	if err != nil {
		return nil, err
	}
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

func (s *Server) tlsKeyCert(key, cert string) (*tls.Config, error) {
	keypair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	//file based tls config using tls defaults
	c := &tls.Config{
		Certificates: []tls.Certificate{keypair},
	}
	return c, nil
}
