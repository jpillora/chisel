package chserver

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/jpillora/chisel/share/craveauth"
)

var AMS_COOKIE_NAME = "_acp_at"

func (s *Server) getCookieHandler(w http.ResponseWriter, r *http.Request) []byte {
	// Retrieve the cookie from the request using its name.
	// If no matching cookie is found, this will return a
	// http.ErrNoCookie error. We check for this, and return a 400 Bad Request
	// response to the client.
	cookie, err := r.Cookie(AMS_COOKIE_NAME)
	if err != nil {
		switch {
		case errors.Is(err, http.ErrNoCookie):
			http.Error(w, "auth or cookie not found", http.StatusBadRequest)
		default:
			log.Println(err)
			http.Error(w, "server error", http.StatusInternalServerError)
		}
		return []byte{}
	}

	// Echo out the cookie value in the response body.
	return []byte(cookie.Value)
}

// handleDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) handleDynamicProxy(w http.ResponseWriter, r *http.Request) bool {
	res, _ := httputil.DumpRequest(r, true)

	ua := r.Header.Get("User-Agent")
	host := r.Host
	s.Infof(string(res))
	// Do a client authentication.
	authKey := []byte(r.Header.Get("Authorization"))
	if len(authKey) == 0 {
		authKey = s.getCookieHandler(w, r)
		if len(authKey) == 0 {
			return true
		}
	}
	s.Infof("Authorizing user key: %v, host: %v, ua: %v", string(authKey), host, ua)
	_, err1 := craveauth.ValidateSignedInUser(authKey, ua, host, s.Logger)
	if err1 != nil {
		s.Infof("User accees denied to %v", err1)
		http.Error(w, err1.Error(), http.StatusUnauthorized)
		return true
	}

	path := r.URL.Path
	s.Infof("Got a dynamic proxy path %v", path)
	if strings.HasPrefix(path, "/register") {
		s.createDynamicProxy(w, r)
	} else if strings.HasPrefix(path, "/unregister") {
		s.deleteDynamicProxy(w, r)
	} else {
		// If path is just a hexadecimal, check it is recorded as a dynamic proxy key
		// if so execute it
		s.executeDynamicProxy(w, r)
	}
	return true

}

type ProxyData struct {
	Id     string `json:"id"`
	Target string `json:"target"`
}

func (s *Server) getProxyData(w http.ResponseWriter, r *http.Request, pd *ProxyData) (error, *httputil.ReverseProxy, bool) {
	err := json.NewDecoder(r.Body).Decode(pd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return err, nil, false
	}
	x, y := s.dynamicReverseProxies[pd.Id]
	return err, x, y
}

// createDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) createDynamicProxy(w http.ResponseWriter, r *http.Request) {
	var pd ProxyData

	err, _, ok := s.getProxyData(w, r, &pd)
	if err != nil {
		return
	}
	if ok {
		http.Error(w, s.Errorf("Id (%s) already existing.", pd.Id).Error(), http.StatusBadRequest)
		return
	}
	s.Infof("Creating reverse proxy for %v, target: %v", pd.Id, pd.Target)
	u, err := url.Parse(pd.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if u.Host == "" {
		http.Error(w, s.Errorf("Missing protocol (%s)", u).Error(), http.StatusBadRequest)
		return
	}
	// Create a NewSingleHostReverseProxy with a custom director
	// use the url path as key to save a pointer to the proxy
	// Send 200 - success
	reverseProxy := httputil.NewSingleHostReverseProxy(u)
	//always use proxy host
	reverseProxy.Director = func(r *http.Request) {
		r.URL.Scheme = u.Scheme
		r.URL.Host = u.Host
		r.Host = u.Host
		r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
		r.Header.Set("Origin", u.Scheme+"://"+u.Host)

		path := r.URL.Path
		path = strings.TrimPrefix(path, "/")
		pathParts := strings.SplitN(path, "/", 2)
		// s.Infof("Setting path : %v %v", path, pathParts)
		if len(pathParts) >= 2 && pathParts[1] != "" {
			path = "/" + pathParts[1]
		} else {
			path = "/"
		}
		// s.Infof("Setting path : %v", path)
		r.URL.Path = path
		s.Infof("Redirecting request to %s at %s\n", r.URL, time.Now().UTC())
	}
	s.dynamicReverseProxies[pd.Id] = reverseProxy
}

// deleteDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) deleteDynamicProxy(w http.ResponseWriter, r *http.Request) {
	var pd ProxyData

	err, _, ok := s.getProxyData(w, r, &pd)
	if err != nil {
		return
	}
	if !ok {
		http.Error(w, s.Errorf("Could not find Id (%s).", pd.Id).Error(), http.StatusBadRequest)
		return
	}
	// just delete the proxy element
	s.Infof("Deleting reverse proxy for %v", pd.Id)
	delete(s.dynamicReverseProxies, pd.Id)
}

// executeDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) executeDynamicProxy(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/")
	pathParts := strings.SplitN(path, "/", 2)
	proxyId := pathParts[0]
	// just serve the reverse proxy request.
	if proxy, ok := s.dynamicReverseProxies[proxyId]; ok {
		proxy.ServeHTTP(w, r)
	} else {
		http.Error(w, s.Errorf("Invalid id (%s)", proxyId).Error(), http.StatusBadRequest)
	}
}
