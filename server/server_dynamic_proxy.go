package chserver

import (
	"crypto/md5"
	"encoding/json"
	"errors"

	//"errors"
	"fmt"
	//"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/jpillora/chisel/share/craveauth"
)

var AMS_COOKIE_NAME = "_acp_at"
var REGISTER_ENDPOINT = "register"
var UNREGISTER_ENDPOINT = "unregister"

func (s *Server) getCookieHandler(r *http.Request) (cookieBytes []byte, err error) {
	// Retrieve the cookie from the request using its name.
	// If no matching cookie is found, this will return a
	// http.ErrNoCookie error. We check for this, and return a 400 Bad Request
	// response to the client.
	cookie, err := r.Cookie(AMS_COOKIE_NAME)
	if err != nil {
		return
	}
	// Echo out the cookie value in the response body.

	cookieBytes = []byte(cookie.Value)
	return
}

func (s *Server) getAuthorizationCookie(r *http.Request) (authKey []byte, err error) { // s.Infof(string(res))
	// Do a client authentication.
	authKey = []byte(r.Header.Get("Authorization"))
	if len(authKey) == 0 {
		authKey, err = s.getCookieHandler(r)
		if err != nil {
			return
		}
	}
	return
}

func (s *Server) authRequest(r *http.Request, useCache bool, target string) (userId int64, authKey []byte, err error) {

	var rHost, rPort string
	ua := r.Header.Get("User-Agent")
	host := r.Host
	userId = 0

	authKey, err = s.getAuthorizationCookie(r)
	if err != nil {
		s.Infof("Auth error : %v", err)
		return
	}

	userId, err = craveauth.ValidateSignedInUser(authKey, ua, host, s.Logger)
	if err != nil {
		s.Infof("User access denied to %v", err)
		return
	}

	u, _ := url.Parse(target)
	rHost, rPort, _ = net.SplitHostPort(u.Host)
	s.Infof("checking access to port %s:%s:%v ", rHost, rPort, userId)
	// if url was ip:port, both rhost and rport would be filled.
	if len(rHost) > 0 && len(rPort) > 0 {
		var allowed bool
		allowed, err = craveauth.CheckTargetUser(rHost, rPort, fmt.Sprint(userId), s.Logger)
		if !allowed {
			s.Infof("Access to port %s:%s:%v denied.", rHost, rPort, userId)
			err = errors.New("Access to requested resource denied.")
			return
		}
		if err != nil {
			s.Infof("Access to port %s:%s:%v denied err: %v", rHost, rPort, userId, err)
			return
		}
	}
	return
}

// handleDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) handleDynamicProxy(w http.ResponseWriter, r *http.Request) bool {
	var pathPrefix string
	// res, _ := httputil.DumpRequest(r, true)
	if strings.HasPrefix(r.URL.Path, "/") {
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) > 1 {
			pathPrefix = pathParts[1]
		}
	}

	s.Infof("Got a dynamic proxy path %v", pathPrefix)
	switch pathPrefix {
	case REGISTER_ENDPOINT:
		s.createDynamicProxy(w, r)
	case UNREGISTER_ENDPOINT:
		s.deleteDynamicProxy(w, r)
	default:
		s.executeDynamicProxy(w, r)
	}
	return true
}

type ProxyData struct {
	Target string `json:"target"`
}

type ProxyRegisterResponse struct {
	Id string `json:"Id"`
}

func (s *Server) getProxyData(w http.ResponseWriter, r *http.Request, pd *ProxyData) (err error) {
	err = json.NewDecoder(r.Body).Decode(pd)
	return
}

func (s *Server) getProxyHashFromTarget(target string) (hash string) {
	hash = fmt.Sprintf("%x", md5.Sum([]byte(target)))
	return
}

// createDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) createDynamicProxy(w http.ResponseWriter, r *http.Request) {
	var pd ProxyData

	err := s.getProxyData(w, r, &pd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.Infof("Creating reverse proxy for %v, target: %v", pd.Target)
	userId, authKey, err := s.authRequest(r, false, pd.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
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
		s.Infof("Setting path : %v %v", path, pathParts)
		if len(pathParts) >= 2 && pathParts[1] != "" {
			path = "/" + pathParts[1]
		} else {
			path = "/"
		}
		s.Infof("Setting path : %v", path)
		r.URL.Path = path
		s.Infof("Redirecting request to %s at %s\n", r.URL, time.Now().UTC())
	}

	pId := s.getProxyHashFromTarget(pd.Target)
	s.dynamicReverseProxies[pId] = &DynamicReverseProxy{
		Handler: reverseProxy,
		AuthKey: authKey,
		Target:  pd.Target,
		User:    userId,
	}

	w.Header().Set("Content-Type", "application/json")
	prr := ProxyRegisterResponse{Id: pId}
	err = json.NewEncoder(w).Encode(prr)
	if err != nil {
		http.Error(w, fmt.Sprintf("error building the response, %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// deleteDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) deleteDynamicProxy(w http.ResponseWriter, r *http.Request) {
	var pd ProxyData

	err := s.getProxyData(w, r, &pd)
	if err != nil {
		return
	}

	pId := s.getProxyHashFromTarget(pd.Target)
	s.Infof("Deleting reverse proxy for %v", pId)
	_, _, err = s.authRequest(r, false, pd.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	delete(s.dynamicReverseProxies, pId)
}

// executeDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) executeDynamicProxy(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/")
	pathParts := strings.SplitN(path, "/", 2)
	proxyId := pathParts[0]

	//just serve the reverse proxy request.
	if proxy, ok := s.dynamicReverseProxies[proxyId]; ok {
		_, _, err := s.authRequest(r, false, proxy.Target)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		proxy.Handler.ServeHTTP(w, r)

	} else {
		http.Error(w, s.Errorf("Invalid id (%s)", proxyId).Error(), http.StatusBadRequest)
	}
}
