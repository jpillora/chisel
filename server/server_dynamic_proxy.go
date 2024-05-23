package chserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
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
			http.Error(w, "auth or cookie not found", http.StatusUnauthorized)
		default:
			log.Println(err)
			http.Error(w, "server error", http.StatusInternalServerError)
		}
		return []byte{}
	}

	// Echo out the cookie value in the response body.
	return []byte(cookie.Value)
}

func (s *Server) authorizeRequest(w http.ResponseWriter, r *http.Request, op, proxyId, proxyTarget string) (int64, []byte) {
	var userId int64
	var rHost, rPort string
	ua := r.Header.Get("User-Agent")
	host := r.Host
	userId = 0

	// s.Infof(string(res))
	// Do a client authentication.
	authKey := []byte(r.Header.Get("Authorization"))
	if len(authKey) == 0 {
		authKey = s.getCookieHandler(w, r)
		if len(authKey) == 0 {
			s.Infof("Authorization key not found.")
			http.Error(w, s.Errorf("Authorization key not found.").Error(), http.StatusUnauthorized)
			return userId, nil
		}
	}

	proxy, proxyExists := s.dynamicReverseProxies[proxyId]
	// s.Infof("Authorizing user key: %v, host: %v, ua: %v", string(authKey), host, ua)
	if op != "/register" && op != "/unregister" {
		// check cache then authorize and return key
		cachedAuthKey := []byte{}
		if proxyExists {
			cachedAuthKey = proxy.AuthKey
		}
		if len(cachedAuthKey) != 0 && string(cachedAuthKey) == string(authKey) {
			s.Infof("Cached auth key matched for proxy target %v.", proxyId)
			userId = proxy.User
		}
	}
	// cache auth key doesnt match requires an auth
	// if cache auth key matches, just do a check on the ip
	if userId == int64(0) {
		uid, err1 := craveauth.ValidateSignedInUser(authKey, ua, host, s.Logger)
		if err1 != nil {
			s.Infof("User access denied to %v", err1)
			http.Error(w, err1.Error(), http.StatusUnauthorized)
			return uid, nil
		}
		userId = uid
	}

	u, _ := url.Parse(proxyTarget)
	rHost, rPort, _ = net.SplitHostPort(u.Host)
	s.Infof("checking access to port %s:%s:%v ", rHost, rPort, userId)
	// if url was ip:port, both rhost and rport would be filled.
	if len(rHost) > 0 && len(rPort) > 0 {
		allowed, err := craveauth.CheckTargetUser(rHost, rPort, fmt.Sprint(userId), s.Logger)
		if !allowed || err != nil {
			s.Infof("access to port %s:%s:%v denied err: %v", rHost, rPort, userId, err)
			err1 := s.Errorf("access to port %s:%s:%v denied err: %v", rHost, rPort, userId, err)
			http.Error(w, err1.Error(), http.StatusUnauthorized)
			return userId, nil
		}
	}
	if op != "/register" && op != "/unregister" {
		s.dynamicReverseProxies[proxyId].AuthKey = authKey
		// s.dynamicReverseProxies[proxyId].Target = proxyTarget
	}
	return userId, authKey
}

// handleDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) handleDynamicProxy(w http.ResponseWriter, r *http.Request) bool {
	// res, _ := httputil.DumpRequest(r, true)
	path := r.URL.Path
	// s.Infof("Got a dynamic proxy path %v", path)
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

func (s *Server) getProxyData(w http.ResponseWriter, r *http.Request, pd *ProxyData) (error, *DynamicReverseProxy, bool) {
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
	userId, authKey := s.authorizeRequest(w, r, "/register", pd.Id, pd.Target)
	if authKey == nil {
		return
	}
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
	s.dynamicReverseProxies[pd.Id] = &DynamicReverseProxy{
		Handler: reverseProxy,
		AuthKey: authKey,
		Target:  pd.Target,
		User:    userId,
	}
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
	_, authKey := s.authorizeRequest(w, r, "/unregister", pd.Id, pd.Target)
	if authKey == nil {
		return
	}
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
		_, authKey := s.authorizeRequest(w, r, proxyId, proxyId, proxy.Target)
		if authKey == nil {
			return
		}
		proxy.Handler.ServeHTTP(w, r)
	} else {
		http.Error(w, s.Errorf("Invalid id (%s)", proxyId).Error(), http.StatusBadRequest)
	}
}
