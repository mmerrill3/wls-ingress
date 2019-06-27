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
	"github.com/mmerrill3/wls-ingress/internal/ingress/annotations"
	apiv1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Configuration holds the definition of all the parts required to describe all
// ingresses reachable by the ingress controller (using a filter by namespace)
type Configuration struct {
	// Backends are a list of backends used by all the Ingress rules in the
	// ingress controller. This list includes the default backend
	Backends []*Backend `json:"backends,omitempty"`

	// ControllerPodsCount contains the list of running ingress controller Pod(s)
	ControllerPodsCount int `json:"controllerPodsCount,omitempty"`
}

// Backend describes one or more remote server/s (endpoints) associated with a service
// +k8s:deepcopy-gen=true
type Backend struct {
	// Name represents an unique apiv1.Service name formatted as <namespace>-<name>-<port>
	Name    string             `json:"name"`
	Service *apiv1.Service     `json:"service,omitempty"`
	Port    intstr.IntOrString `json:"port"`
	// Endpoints contains the list of endpoints currently running
	Endpoints []Endpoint `json:"endpoints,omitempty"`
}

// Endpoint describes a kubernetes endpoint in a backend
// +k8s:deepcopy-gen=true
type Endpoint struct {
	// Address IP address of the endpoint
	Address string `json:"address"`
	// Port number of the TCP port
	Port string `json:"port"`
	// Target returns a reference to the object providing the endpoint
	Target *apiv1.ObjectReference `json:"target,omitempty"`
}

// Ingress holds the definition of an Ingress plus its annotations
type Ingress struct {
	networking.Ingress
	ParsedAnnotations *annotations.Ingress
}
