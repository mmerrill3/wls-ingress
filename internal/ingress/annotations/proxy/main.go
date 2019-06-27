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

package proxy

import (
	networking "k8s.io/api/networking/v1beta1"

	"github.com/mmerrill3/wls-ingress/internal/ingress/annotations/parser"
	"github.com/mmerrill3/wls-ingress/internal/ingress/resolver"
)

// Config returns the proxy timeout to use in the upstream server/s
type Config struct {
	BodySize    string `json:"bodySize"`
	ReadTimeout int    `json:"readTimeout"`
}

// Equal tests for equality between two Configuration types
func (l1 *Config) Equal(l2 *Config) bool {
	if l1 == l2 {
		return true
	}
	if l1 == nil || l2 == nil {
		return false
	}
	if l1.BodySize != l2.BodySize {
		return false
	}
	if l1.ReadTimeout != l2.ReadTimeout {
		return false
	}

	return true
}

type proxy struct {
	r resolver.Resolver
}

// NewParser creates a new reverse proxy configuration annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return proxy{r}
}

// ParseAnnotations parses the annotations contained in the ingress
// rule used to configure upstream check parameters
func (a proxy) Parse(ing *networking.Ingress) (interface{}, error) {
	config := &Config{}

	var err error

	config.ReadTimeout, err = parser.GetIntAnnotation("proxy-read-timeout", ing)
	if err != nil {
		config.ReadTimeout = 300
	}

	config.BodySize, err = parser.GetStringAnnotation("proxy-body-size", ing)
	if err != nil {
		config.BodySize = "10m"
	}

	return config, nil
}
