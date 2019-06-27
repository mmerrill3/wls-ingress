/*
Copyright 2015 The Kubernetes Authors.
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

package controller

import (
	"time"

	wls_config "github.com/mmerrill3/wls-ingress/internal/ingress/controller/config"
	clientset "k8s.io/client-go/kubernetes"
)

const (
	defUpstreamName = "upstream-default-backend"
	defServerName   = "_"
	rootLocation    = "/"
)

// Configuration contains all the settings required by an Ingress controller
type Configuration struct {
	APIServerHost  string
	KubeConfigFile string
	Client         clientset.Interface
	ResyncPeriod   time.Duration
	DefaultService string
	Namespace      string
	ListenPorts    *wls_config.ListenPorts
	EnableMetrics  bool
}
