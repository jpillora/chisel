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

	"github.com/jpillora/chisel/dcrpc"
	"github.com/jpillora/chisel/share/craveauth"
	"google.golang.org/grpc"
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

func (s *Server) getAuthorizationCookie(r *http.Request) (authKey []byte, err error) {
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

func (s *Server) disconnectResourceDcMaster(drProxy *DynamicReverseProxy) {
	drProxy.GrpcConn.Close()
}

// Authorize user to the target, ideally sets the connection.
func (s *Server) checkResourceAvailableDcMaster(drProxy *DynamicReverseProxy, pId string, createNew bool) (err error) {
	var client dcrpc.DcMasterRPCClient
	u, _ := url.Parse(drProxy.Target)
	ip, _, _ := net.SplitHostPort(u.Host)

	if createNew {
		var conn *grpc.ClientConn
		s.Infof("Connecting to resource host %s.", ip)
		client, conn, err = craveauth.ConnectDCMasterRPC(ip, s.Logger)
		if err != nil {
			return
		}
		drProxy.DcMasterClient = client
		drProxy.GrpcConn = conn
	} else {
		// s.Infof("Checking availability of resource ip: %s, job: %v.", ip, drProxy.JobId)
		err = craveauth.CheckForJob(drProxy.DcMasterClient, pId, drProxy.JobId)
		if err != nil {
			err = s.Errorf("Resource unavailable. Error: %v", err)
			s.Infof("%v", err)
			return
		}
		s.Infof("Available resource ip: %s, job: %v.", ip, drProxy.JobId)
	}
	return
}

func (s *Server) checkResourceAccessNoop(drProxy *DynamicReverseProxy) (err error) {
	return
}

// Authenticate user to the target, ideally sets the jobid.
func (s *Server) checkResourceAccessDcMaster(drProxy *DynamicReverseProxy) (err error) {
	u, _ := url.Parse(drProxy.Target)
	rHost, rPort, _ := net.SplitHostPort(u.Host)
	// if url was ip:port, both rhost and rport would be filled.
	if len(rHost) > 0 && len(rPort) > 0 {
		var allowed bool
		var jobId int64

		// s.Infof("Checking access to resource %s:%s for user: %v", rHost, rPort, drProxy.User)
		jobId, allowed, err = craveauth.CheckTargetUser(rHost, rPort, fmt.Sprint(drProxy.User), s.Logger)
		if !allowed {
			s.Infof("Access to resource %s:%s for user: %v denied.", rHost, rPort, drProxy.User)
			err = errors.New("Access to requested resource denied.")
			return
		}
		if err != nil {
			s.Infof("Access to resource %s:%s for user: %v denied. Error: %v", rHost, rPort, drProxy.User, err)
			return
		}
		s.Infof("Granted access to resource %s:%s for user: %v, job: %v", rHost, rPort, drProxy.User, jobId)
		drProxy.JobId = jobId
	}
	return
}

// Authenticate user to the request, ideally sets the userid and authkey.
func (s *Server) authRequest(r *http.Request, useCache bool, drProxy *DynamicReverseProxy,
	checkResourceAccess func(drProxy *DynamicReverseProxy) error) (err error) {
	var userId int64

	authKey, err := s.getAuthorizationCookie(r)
	if err != nil {
		s.Infof("Authkey error: %v", err)
		return
	}

	// if useCache, match cookie, else validate
	// false for register and unregister, so a reverse proxy should exist.
	if useCache {
		if string(authKey) == string(drProxy.AuthKey) {
			return
		}
	}

	// user id is not set through because cache did not match or useCache is false
	// go ahead and access db to get user id and jobid
	userId, err = craveauth.ValidateSignedInUser(authKey, r.Header.Get("User-Agent"), r.Host, s.Logger)
	if err != nil {
		s.Infof("User access denied. Error: %v", err)
		return
	}
	drProxy.User = userId
	drProxy.AuthKey = authKey

	return checkResourceAccess(drProxy)
}

// handleDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) handleDynamicProxy(w http.ResponseWriter, r *http.Request) (handled bool) {
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
		handled = true
	case UNREGISTER_ENDPOINT:
		s.deleteDynamicProxy(w, r)
		handled = true
	default:
		handled = s.executeDynamicProxy(w, r)
	}
	return
}

type ProxyData struct {
	Target string `json:"target"`
}

type ProxyRegisterResponse struct {
	Id string `json:"id"`
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
	var drProxy DynamicReverseProxy

	err := s.getProxyData(w, r, &pd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	drProxy.Target = pd.Target
	s.Infof("Creating reverse proxy for target: %v", pd.Target)
	err = s.authRequest(r, false, &drProxy, s.checkResourceAccessDcMaster)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	pId := s.getProxyHashFromTarget(pd.Target)
	err = s.checkResourceAvailableDcMaster(&drProxy, pId, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
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
		// s.Infof("Setting path : %v %v", path, pathParts)
		if len(pathParts) >= 2 && pathParts[1] != "" {
			path = "/" + pathParts[1]
		} else {
			path = "/"
		}
		s.Infof("Setting path : %v", path)
		r.URL.Path = path
		s.Infof("Redirecting request to %s at %s\n", r.URL, time.Now().UTC())
	}
	drProxy.Handler = reverseProxy
	s.dynamicReverseProxies[pId] = &drProxy

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
	if proxy, ok := s.dynamicReverseProxies[pId]; ok {
		s.Infof("Deleting reverse proxy for %v", pId)
		err = s.authRequest(r, false, proxy, s.checkResourceAccessNoop)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		s.disconnectResourceDcMaster(proxy)
		delete(s.dynamicReverseProxies, pId)
	} else { // do we need this error?
		http.Error(w, s.Errorf("Invalid id (%s)", pId).Error(), http.StatusBadRequest)
	}
	return
}

// executeDynamicProxy is the main http websocket handler for the chisel server
func (s *Server) executeDynamicProxy(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/")
	pathParts := strings.SplitN(path, "/", 2)
	pId := pathParts[0]

	//just serve the reverse proxy request.
	if proxy, ok := s.dynamicReverseProxies[pId]; ok {
		err := s.authRequest(r, true, proxy, s.checkResourceAccessDcMaster)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return ok
		}
		err = s.checkResourceAvailableDcMaster(proxy, pId, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return ok
		}
		proxy.Handler.ServeHTTP(w, r)
		return ok
	}
	return false
}
