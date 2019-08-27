package chshare

import (
	"golang.org/x/net/proxy"
	"net"
	"net/url"
)

type FnDial func(network, addr string) (net.Conn, error)

func NewSocks5Dial(proxyURL *url.URL) (FnDial, error) {
	var auth *proxy.Auth = nil
	if proxyURL.User != nil {
		pass, _ := proxyURL.User.Password()
		auth = &proxy.Auth{
			User:     proxyURL.User.Username(),
			Password: pass,
		}
	}
	socksDialer, err := proxy.SOCKS5("tcp", proxyURL.Host, auth, proxy.Direct)
	if err != nil {
		return nil, err
	}
	return socksDialer.Dial, nil
}
