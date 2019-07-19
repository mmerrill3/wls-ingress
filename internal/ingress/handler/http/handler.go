/*
Copyright 2019 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package http

import (
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/mmerrill3/wls-ingress/internal/ingress"
	"github.com/mmerrill3/wls-ingress/internal/ingress/controller/config"
	"github.com/mmerrill3/wls-ingress/internal/redis"
	"k8s.io/klog"
)

//WLSHandler handles http traffic for wls
type WLSHandler struct {
	proxyMap      sync.Map
	lookupService *redis.Manager
	targetConfig  *config.Configuration
	lock          sync.Mutex
}

// InstallHandler registers handlers for http proxying to wls
func InstallHandler(mux *http.ServeMux, config *config.Configuration, ls *redis.Manager) (*WLSHandler, error) {
	handler := &WLSHandler{
		targetConfig:  config,
		lock:          sync.Mutex{},
		lookupService: ls,
	}
	mux.Handle("/", http.HandlerFunc(handler.handleRootWLS()))
	return handler, nil
}

// UpdateConfiguration updates the running config within the http server mux.
func (h *WLSHandler) UpdateConfiguration(config *config.Configuration) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.targetConfig = config
}

func (h *WLSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handleRootWLS()(w, r)
}

// handleRootWLS returns an http.HandlerFunc that serves everything.
func (h *WLSHandler) handleRootWLS() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		klog.V(4).Infof("Received request %v - %v - %v", r.URL, r.Header, r.Body)
		//get the host
		host := h.getHost(r)
		if host == "" {
			klog.Error("No Host Error")
			http.Error(w, "No Host Error", http.StatusBadRequest)
			return
		}
		endpoints := h.getEndpoints(host)
		if endpoints == nil {
			klog.Error("No endpoints Error")
			http.Error(w, "No endpoints Error", http.StatusNotFound)
			r.Close = true
			return
		}
		if len(endpoints) == 0 {
			klog.Infof("Error looking up the upstream for host %v - nothing defined yet", host)
			http.Error(w, "No Services Error", http.StatusServiceUnavailable)
			r.Close = true
			return
		}
		//check the header
		cookie, err := r.Cookie("JSESSIONID")
		if err != nil {
			if err != http.ErrNoCookie {
				klog.Errorf("Error getting cookie from request - %v", r)
				http.Error(w, "Cookie Error", http.StatusBadRequest)
				r.Close = true
				return
			}
			//send to random upstream, and get the cookie that comes back
			randomIndex := rand.Intn(len(endpoints))
			if val, ok := h.proxyMap.Load(endpoints[randomIndex].Address); !ok {
				{
					url, err := url.Parse("http://" + endpoints[randomIndex].Address + ":" + endpoints[randomIndex].Port)
					if err != nil {
						klog.Errorf("Error creating the url - %v", err)
						http.Error(w, "URL Error", http.StatusInternalServerError)
						return
					}
					h.proxyMap.Store(endpoints[randomIndex].Address, httputil.NewSingleHostReverseProxy(url))
				}
				p, _ := h.proxyMap.Load(endpoints[randomIndex].Address)
				proxyType := p.(*httputil.ReverseProxy)
				proxyType.ServeHTTP(w, r)
			} else {
				valType := val.(*httputil.ReverseProxy)
				valType.ServeHTTP(w, r)
			}
			//get the cookie from the response set-cookie header
			klog.V(4).Infof("Headers map in response %+v", w.Header())
			cookie := w.Header().Get("Set-Cookie")
			klog.V(4).Infof("Found cookie %+v", cookie)
			if cookie == "" {
				klog.Errorf("Error creating the new conection - %v", err)
				http.Error(w, "URL Error", http.StatusInternalServerError)
				return
			}
			//get the cookie value for JSESSIONID
			if strings.Index(cookie, "JSESSIONID=") >= 0 {
				jsession := strings.Split(strings.Split(cookie, "JSESSIONID=")[1], ";")
				h.lookupService.AddCookie(jsession[0], endpoints[randomIndex].Address)
			} else {
				klog.Error("Error creating the new conection couldn't find the Set-Cookie")
				http.Error(w, "URL Error", http.StatusInternalServerError)
				return
			}
			return
		}
		//the cookie is there, look it up from redis
		klog.V(4).Infof("looking up cookie %v", cookie.Value)
		endpoint, err := h.lookupService.GetEndpoint(cookie.Value, true)
		if err != nil {
			klog.Errorf("Error looking up the upstream for cookie %v - %v", cookie, err)
			http.Error(w, "Redis Error", http.StatusInternalServerError)
			return
		}
		if endpoint == "" {
			klog.Infof("Error looking up the upstream for cookie %v - its gone", cookie)
			http.Error(w, "Upstream Service Went Away Error", http.StatusServiceUnavailable)
			r.Close = true
			return
		}
		if val, ok := h.proxyMap.Load(endpoint); !ok {
			klog.Infof("Adding the upstream %v to the map", endpoint)
			url, err := url.Parse("http://" + endpoint + ":7001")
			if err != nil {
				klog.Errorf("Error creating the url - %v", err)
				http.Error(w, "URL Error", http.StatusInternalServerError)
				return
			}
			h.proxyMap.Store(endpoint, httputil.NewSingleHostReverseProxy(url))
			p, _ := h.proxyMap.Load(endpoint)
			pType := p.(*httputil.ReverseProxy)
			pType.ServeHTTP(w, r)
		} else {
			valType := val.(*httputil.ReverseProxy)
			valType.ServeHTTP(w, r)
		}
	})
}

// removeEntry removes the existing reverse proxy in the map
func (h *WLSHandler) removeProxy(upstream string) error {
	h.proxyMap.Delete(upstream)
	return nil
}

//TODO when an endpoint goes away, remove the reverse proxy

func (h *WLSHandler) getHost(r *http.Request) string {
	if r.Host == "" {
		klog.Warning("the request does not contain a host header")
		return ""
	}
	hostPieces := strings.Split(r.Host, ":")
	return hostPieces[0]
}

func (h *WLSHandler) getEndpoints(host string) []ingress.Endpoint {
	h.lock.Lock()
	services := h.targetConfig.Services
	h.lock.Unlock()
	if services == nil {
		klog.Warning("no services defined yet")
		return nil
	}
	for _, service := range services {
		if service.Hostname == host {
			return service.Backend.Endpoints
		}
	}
	klog.Warningf("no services defined yet that match host %v", host)
	return nil
}
