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

	"github.com/mmerrill3/wls-ingress/internal/redis"
	"k8s.io/klog"
)

//WLSHandler handles http traffic for wls
type WLSHandler struct {
	proxyMap sync.Map
	//TODO protect the load upstreams with the mutex
	//mut           sync.Mutex
	lookupService *redis.Manager
	upstreams     []string
}

// InstallHandler registers handlers for http proxying to wls
func InstallHandler(mux *http.ServeMux) (*WLSHandler, error) {
	handler := &WLSHandler{}
	mux.Handle("/", http.HandlerFunc(handler.handleRootWLS()))
	return handler, nil
}

func (h *WLSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handleRootWLS()(w, r)
}

// handleRootWLS returns an http.HandlerFunc that serves everything.
func (h *WLSHandler) handleRootWLS() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//check the header
		cookie, err := r.Cookie("JESSIONID")
		if err != nil {
			if err != http.ErrNoCookie {
				klog.Errorf("Error getting cookie from request - %v", r)
				createErrorResponse("Cookie Error", w)
				return
			}
			//send to random upstream, and get the cookie that comes back
			randomIndex := rand.Intn(len(h.upstreams))
			if val, ok := h.proxyMap.Load(h.upstreams[randomIndex]); !ok {
				{
					url, err := url.Parse("http://" + h.upstreams[randomIndex] + ":7001")
					if err != nil {
						klog.Errorf("Error creating the url - %v", err)
						createErrorResponse("URL Error", w)
						return
					}
					h.proxyMap.Store(h.upstreams[randomIndex], httputil.NewSingleHostReverseProxy(url))
				}
				p, _ := h.proxyMap.Load(h.upstreams[randomIndex])
				proxyType := p.(*httputil.ReverseProxy)
				proxyType.ServeHTTP(w, r)
			} else {
				valType := val.(*httputil.ReverseProxy)
				valType.ServeHTTP(w, r)
			}
			//get the cookie from the response set-cookie header
			cookie, err := r.Cookie("Set-Cookie")
			if err != nil {
				klog.Errorf("Error creating the new conection - %v", err)
				createErrorResponse("URL Error", w)
				return
			}
			//get the cookie value for JSESSIONID
			if strings.Index(cookie.Value, "JSESSIONID=") > 0 {
				jsession := strings.SplitAfterN(cookie.Value, "JSESSIONID=", 1)
				h.lookupService.AddCookie(jsession[0], h.upstreams[randomIndex])
			} else {
				klog.Error("Error creating the new conection couldn't find the Set-Cookie")
				createErrorResponse("URL Error", w)
				return
			}
			return
		}
		//the cookie is there, look it up from redis
		endpoint, err := h.lookupService.GetEndpoint(cookie.Value)
		if err != nil {
			klog.Errorf("Error lookup up the upstream for cookie %v - %v", cookie, err)
			createErrorResponse("Redis Error", w)
			return
		}
		if val, ok := h.proxyMap.Load(endpoint); !ok {
			klog.Infof("Adding the upstream %v to the map", endpoint)
			url, err := url.Parse("http://" + endpoint + ":7001")
			if err != nil {
				klog.Errorf("Error creating the url - %v", err)
				createErrorResponse("URL Error", w)
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

func (h *WLSHandler) addUpstream(upstream string) {
	h.upstreams = append(h.upstreams, upstream)
}

func (h *WLSHandler) deleteUpstream(upstream string) {
	for i, upstreamLocal := range h.upstreams {
		if upstreamLocal == upstream {
			h.upstreams[len(h.upstreams)-1], h.upstreams[i] = h.upstreams[i], h.upstreams[len(h.upstreams)-1]
			h.upstreams = h.upstreams[:len(h.upstreams)-1]
			break
		}
	}
}

func createErrorResponse(message string, w http.ResponseWriter) {
	http.Error(w, message, 500)
}
