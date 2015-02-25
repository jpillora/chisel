package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/jpillora/chisel"
	"golang.org/x/net/websocket"
)

func main() {

	auth := os.Getenv("AUTH")

	handshakeWS := func(h string) (*chisel.Config, error) {
		c, err := chisel.DecodeConfig(h)
		if err != nil {
			return nil, err
		}
		if chisel.Version != c.Version {
			return nil, fmt.Errorf("Version mismatch")
		}
		if auth != "" {
			if auth != c.Auth {
				return nil, fmt.Errorf("Authentication failed")
			}
		}
		return c, nil
	}

	handleWS := websocket.Handler(func(ws *websocket.Conn) {
		protos := ws.Config().Protocol
		if len(protos) != 1 {
			ws.Write([]byte("Handshake invalid"))
			ws.Close()
			return
		}
		config, err := handshakeWS(protos[0])
		if err != nil {
			ws.Write([]byte("Handshake denied: " + err.Error()))
			ws.Close()
			return
		}
		fmt.Printf("%+v\n", config)
		io.Copy(ws, ws)
	})

	handleHTTP := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "websocket" {
			handleWS.ServeHTTP(w, r)
		} else {
			w.WriteHeader(200)
			w.Write([]byte("hello world\n"))
		}
	})

	//get port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	//listen
	log.Println("listening on " + port)
	log.Fatal(http.ListenAndServe(":"+port, handleHTTP))
}
