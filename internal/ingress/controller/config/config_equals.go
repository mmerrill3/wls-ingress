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

package config

import (
	"github.com/mmerrill3/wls-ingress/internal/sets"
)

// Equal tests for equality between two Configuration types
func (c1 *Configuration) Equal(c2 *Configuration) bool {
	if c1 == c2 {
		return true
	}
	if c1 == nil || c2 == nil {
		return false
	}

	match := compareServices(c1.Services, c2.Services)
	if !match {
		return false
	}

	return true
}

// Equal tests for equality between two ConfigurationBackend types
func (s1 *ConfigurationBackend) Equal(s2 *ConfigurationBackend) bool {
	if s1 == s2 {
		return true
	}
	if s1 == nil || s2 == nil {
		return false
	}
	if s1.Hostname != s2.Hostname {
		return false
	}
	if !(s1.Backend).Equal(s2.Backend) {
		return false
	}
	return true
}

var compareServicesFunc = func(e1, e2 interface{}) bool {
	b1, ok := e1.(*ConfigurationBackend)
	if !ok {
		return false
	}

	b2, ok := e2.(*ConfigurationBackend)
	if !ok {
		return false
	}

	return b1.Equal(b2)
}

func compareServices(a, b []*ConfigurationBackend) bool {
	return sets.Compare(a, b, compareServicesFunc)
}
