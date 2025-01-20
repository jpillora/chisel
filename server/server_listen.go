package server

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
	tlsConf, listener, err2 := s.createTLSConf(port)
	if err2 != nil {
		return listener, err2
	}
	//tcp listen
	l, n, err3 := s.listenHTTP(host, port, tlsConf)
	if err3 != nil {
		return n, err3
	}
	return l, nil
}

func (s *Server) createTLSConf(port string) (*tls.Config, net.Listener, error) {
	hasKeyCert := s.config.TLS.Key != "" && s.config.TLS.Cert != ""
	var tlsConf *tls.Config
	if hasKeyCert {
		c, err := s.tlsKeyCert(s.config.TLS.Key, s.config.TLS.Cert)
		if err != nil {
			return nil, nil, err
		}
		tlsConf = c

		if port != "443" {
			s.Debugf("consider using port 443 with TLS connection; Should not be used with production")
		}
	}
	return tlsConf, nil, nil
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

func (s *Server) listenHTTP(host string, port string, tlsConf *tls.Config) (net.Listener, net.Listener, error) {
	l, err := net.Listen("tcp", host+":"+port)
	if err != nil {
		return nil, nil, err
	}
	proto := "http"
	if tlsConf != nil {
		proto += "s"
		l = tls.NewListener(l, tlsConf)
	}
	if err == nil {
		s.Infof("Listening on %s://%s:%s", proto, host, port)
	}
	return l, nil, nil
}
