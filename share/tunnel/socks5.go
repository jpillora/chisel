package tunnel

import (
	"log"
	"strings"

	"github.com/armon/go-socks5"
	"golang.org/x/net/context"
)

type Socks5Config struct {
	Auth         string
	AllowedHosts []string
}

type Socks5AllowedHosts struct {
	AllowedHosts map[string]struct{}
}

func (p *Socks5AllowedHosts) Allow(ctx context.Context, req *socks5.Request) (context.Context, bool) {
	// only process FQDN
	_, ok := p.AllowedHosts[req.DestAddr.FQDN]
	return ctx, ok
}

//creates Socks5 server with optional authentication and host filter
func CreateSocks5Server(sl *log.Logger, c Config) (*socks5.Server, error) {
	socksServerConfig := socks5.Config{Logger: sl}
	if c.Socks5Config.Auth != "" {
		if !strings.Contains(c.Socks5Config.Auth, ":") {
			sl.Fatal("Invalid format of socks5-auth. Missing colon separator")
		}
		creds := strings.Split(c.Socks5Config.Auth, ":")
		cred := socks5.StaticCredentials{
			creds[0]: creds[1],
		}
		socksServerConfig.Credentials = cred
	}

	if len(c.Socks5Config.AllowedHosts) > 0 {
		allowedHosts := c.Socks5Config.AllowedHosts
		set := make(map[string]struct{}, len(allowedHosts))
		for _, s := range allowedHosts {
			set[strings.Trim(s, " ")] = struct{}{}
		}

		socksServerConfig.Rules = &Socks5AllowedHosts{set}
	}

	return socks5.New(&socksServerConfig)
}
