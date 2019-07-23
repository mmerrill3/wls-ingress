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

package ingress

import (
	"github.com/mmerrill3/wls-ingress/internal/sets"
)

// Equal checks the equality against an Endpoint
func (e1 *Endpoint) Equal(e2 *Endpoint) bool {
	if e1 == e2 {
		return true
	}
	if e1 == nil || e2 == nil {
		return false
	}
	if e1.Address != e2.Address {
		return false
	}
	if e1.Port != e2.Port {
		return false
	}

	if e1.Target != e2.Target {
		if e1.Target == nil || e2.Target == nil {
			return false
		}
		if e1.Target.UID != e2.Target.UID {
			return false
		}
		if e1.Target.ResourceVersion != e2.Target.ResourceVersion {
			return false
		}
	}

	return true
}

// Equal tests for equality between two Backend types
func (b1 *Backend) Equal(b2 *Backend) bool {
	if b1 == b2 {
		return true
	}
	if b1 == nil || b2 == nil {
		return false
	}
	if b1.Name != b2.Name {
		return false
	}

	if b1.Service != b2.Service {
		if b1.Service == nil || b2.Service == nil {
			return false
		}
		if b1.Service.GetNamespace() != b2.Service.GetNamespace() {
			return false
		}
		if b1.Service.GetName() != b2.Service.GetName() {
			return false
		}
	}

	match := compareEndpoints(b1.Endpoints, b2.Endpoints)
	if !match {
		return false
	}

	return true
}

var compareEndpointsFunc = func(e1, e2 interface{}) bool {
	ep1, ok := e1.(Endpoint)
	if !ok {
		return false
	}

	ep2, ok := e2.(Endpoint)
	if !ok {
		return false
	}

	return (&ep1).Equal(&ep2)
}

func compareEndpoints(a, b []Endpoint) bool {
	return sets.Compare(a, b, compareEndpointsFunc)
}
