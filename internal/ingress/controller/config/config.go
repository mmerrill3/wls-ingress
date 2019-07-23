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
	"github.com/mmerrill3/wls-ingress/internal/ingress"
)

// ListenPorts describe the ports required to run the
// WLS Ingress controller
type ListenPorts struct {
	HTTP    int
	Health  int
	Default int
}

// ConfigurationBackend represents the mapping of hostnames to backend endpoints
type ConfigurationBackend struct {
	// The backend for the defined hostname (what port, what endpoints, what service?)
	Backend *ingress.Backend
	// The hostname of the backend
	Hostname string
}

// Configuration represents all the hostnames and backend this controller managers
type Configuration struct {
	Services []*ConfigurationBackend

	// ControllerPodsCount contains the list of running ingress controller Pod(s)
	ControllerPodsCount int
}
