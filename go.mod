module github.com/jpillora/chisel

go 1.26.5

require (
	github.com/armon/go-socks5 v0.0.0-20160902184237-e75332964ef5
	github.com/fsnotify/fsnotify v1.10.1
	github.com/gorilla/websocket v1.5.3
	github.com/jpillora/backoff v1.0.0
	github.com/jpillora/requestlog v1.0.0
	github.com/jpillora/sizestr v1.0.0
	golang.org/x/crypto v0.53.0
	golang.org/x/net v0.56.0
	golang.org/x/sync v0.21.0
)

require (
	github.com/andrew-d/go-termutil v0.0.0-20150726205930-009166a695a2 // indirect
	github.com/jpillora/ansi v1.0.3 // indirect
	github.com/tomasen/realip v0.0.0-20180522021738-f0c99a92ddce // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
)

replace github.com/jpillora/chisel => ../chisel
