module tunnel

go 1.25

toolchain go1.25.7

require (
	github.com/armon/go-socks5 v0.0.0-20160902184237-e75332964ef5
	github.com/fsnotify/fsnotify v1.9.0
	github.com/gorilla/websocket v1.5.3
	github.com/jpillora/backoff v1.0.0
	github.com/jpillora/sizestr v1.0.0
	golang.org/x/crypto v0.48.0
	golang.org/x/net v0.50.0
	golang.org/x/sync v0.19.0
)

require golang.org/x/sys v0.41.0 // indirect
