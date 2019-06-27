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

package defaults

import "net"

// Backend defines the mandatory configuration that an Ingress controller must provide
// The reason of this requirements is the annotations are generic. If some implementation do not supports
// one or more annotations it just can provides defaults
type Backend struct {
	// AppRoot contains the AppRoot for apps that doesn't exposes its content in the 'root' context
	AppRoot string `json:"app-root"`

	// http://nginx.org/en/docs/http/ngx_http_core_module.html#client_max_body_size
	// Sets the maximum allowed size of the client request body
	ProxyBodySize string `json:"proxy-body-size"`

	// Timeout in seconds for reading a response from the proxied server. The timeout is set only between
	// two successive read operations, not for the transmission of the whole response
	// http://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_read_timeout
	ProxyReadTimeout int `json:"proxy-read-timeout"`

	// Name server/s used to resolve names of upstream servers into IP addresses.
	// The file /etc/resolv.conf is used as DNS resolution configuration.
	Resolver []net.IP
}
