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
	"fmt"
	"strconv"
	"time"

	"github.com/mmerrill3/wls-ingress/internal/ingress"
	wls_config "github.com/mmerrill3/wls-ingress/internal/ingress/controller/config"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

const (
	defUpstreamName = "upstream-default-backend"
	defServerName   = "_"
	rootLocation    = "/"
)

// Configuration contains all the settings required by an Ingress controller
type Configuration struct {
	APIServerHost        string
	KubeConfigFile       string
	Client               clientset.Interface
	ResyncPeriod         time.Duration
	DefaultService       string
	Namespace            string
	ListenPorts          *wls_config.ListenPorts
	EnableMetrics        bool
	ElectionID           string
	PublishService       string
	SyncRateLimit        float32
	RedisSentinelService string
	RedisSentinelPort    int
	RedisMasterName      string
	RedisMaxRetries      int
	RedisMinRetryBackoff time.Duration
	RedisMaxRetryBackoff time.Duration
	RedisNamespacePrefix string
}

// syncIngress collects all the pieces required to assemble the
// configuration
func (n *WLSController) syncIngress(interface{}) error {
	n.syncRateLimiter.Accept()

	if n.syncQueue.IsShuttingDown() {
		return nil
	}

	ings := n.store.ListIngresses(nil)
	generatedConfiguration := n.getConfiguration(ings)

	if n.runningConfig.Equal(generatedConfiguration) {
		klog.V(3).Infof("No configuration change detected, skipping backend reload.")
		return nil
	}

	klog.Infof("Configuration changes detected, backend reload required.")

	err := n.OnUpdate(generatedConfiguration)
	if err != nil {
		klog.Errorf("Unexpected failure reloading the backend:\n%v", err)
		return err
	}

	klog.Infof("Backend successfully reloaded.")

	return nil
}

// getConfiguration returns a configuration of hosts and their upstream services and endpoints
func (n *WLSController) getConfiguration(ingresses []*ingress.Ingress) *wls_config.Configuration {
	klog.V(4).Infof("Getting configuration for ingresses %v", ingresses)
	upstreams := n.createUpstreams(ingresses)
	klog.V(4).Infof("Generated upstreams %v", upstreams)
	servers := n.createServers(ingresses, upstreams)
	klog.V(4).Infof("Generated servers %v", upstreams)
	var cbe []*wls_config.ConfigurationBackend
	for hostname, backend := range servers {
		cbe = append(cbe, &wls_config.ConfigurationBackend{
			Backend:  backend,
			Hostname: hostname,
		})
	}
	configuration := &wls_config.Configuration{
		Services: cbe,
	}
	return configuration
}

// serviceEndpoints returns the upstream servers (Endpoints) associated with a Service.
func (n *WLSController) serviceEndpoints(svcKey, backendPort string) ([]ingress.Endpoint, error) {
	svc, err := n.store.GetService(svcKey)

	var upstreams []ingress.Endpoint
	if err != nil {
		return upstreams, err
	}

	klog.V(3).Infof("Obtaining ports information for Service %q", svcKey)

	// Ingress with an ExternalName Service and no port defined for that Service
	if svc.Spec.Type == apiv1.ServiceTypeExternalName {
		externalPort, err := strconv.Atoi(backendPort)
		if err != nil {
			klog.Warningf("Only numeric ports are allowed in ExternalName Services: %q is not a valid port number.", backendPort)
			return upstreams, nil
		}

		servicePort := apiv1.ServicePort{
			Protocol:   "TCP",
			Port:       int32(externalPort),
			TargetPort: intstr.FromString(backendPort),
		}
		endps := getEndpoints(svc, &servicePort, apiv1.ProtocolTCP, n.store.GetServiceEndpoints)
		if len(endps) == 0 {
			klog.Warningf("Service %q does not have any active Endpoint.", svcKey)
			return upstreams, nil
		}

		upstreams = append(upstreams, endps...)
		return upstreams, nil
	}

	for _, servicePort := range svc.Spec.Ports {
		// targetPort could be a string, use either the port name or number (int)
		if strconv.Itoa(int(servicePort.Port)) == backendPort ||
			servicePort.TargetPort.String() == backendPort ||
			servicePort.Name == backendPort {

			endps := getEndpoints(svc, &servicePort, apiv1.ProtocolTCP, n.store.GetServiceEndpoints)
			if len(endps) == 0 {
				klog.Warningf("Service %q does not have any active Endpoint.", svcKey)
			}

			upstreams = append(upstreams, endps...)
			break
		}
	}

	return upstreams, nil
}

// createUpstreams creates the upstreams (Endpoints) for each Service
// referenced in Ingress rules.
func (n *WLSController) createUpstreams(data []*ingress.Ingress) map[string]*ingress.Backend {
	upstreams := make(map[string]*ingress.Backend)
	for _, ing := range data {
		for _, rule := range ing.Spec.Rules {
			defServiceName := rule.IngressRuleValue.HTTP.Paths[0].Backend.ServiceName
			defServicePort := rule.IngressRuleValue.HTTP.Paths[0].Backend.ServicePort
			defBackend := upstreamName(ing.Namespace, defServiceName, defServicePort)

			klog.V(3).Infof("Creating upstream %q", defBackend)
			upstreams[defBackend] = newUpstream(defBackend)

			svcKey := fmt.Sprintf("%v/%v", ing.Namespace, defServiceName)

			endps, err := n.serviceEndpoints(svcKey, defServicePort.String())
			upstreams[defBackend].Endpoints = append(upstreams[defBackend].Endpoints, endps...)
			if err != nil {
				klog.Warningf("Error creating upstream %q: %v", defBackend, err)
			}

			s, err := n.store.GetService(svcKey)
			if err != nil {
				klog.Warningf("Error obtaining Service %q: %v", svcKey, err)
			}
			upstreams[defBackend].Service = s
		}
	}
	return upstreams
}

// createServers builds a map of host name to Server structs from a map of
// already computed Upstream structs.
func (n *WLSController) createServers(data []*ingress.Ingress,
	upstreams map[string]*ingress.Backend) map[string]*ingress.Backend {

	servers := make(map[string]*ingress.Backend, len(data))

	// initialize all servers
	for _, ing := range data {
		for _, rule := range ing.Spec.Rules {
			host := rule.Host
			if host == "" {
				klog.Errorf("rule without a host is not allowed for now, skipping ing %q", ing.Name)
				continue
			}
			if _, ok := servers[host]; ok {
				// server already configured
				continue
			}
			defUpstream := upstreamName(ing.Namespace, rule.IngressRuleValue.HTTP.Paths[0].Backend.ServiceName, rule.IngressRuleValue.HTTP.Paths[0].Backend.ServicePort)
			servers[host] = upstreams[defUpstream]
		}
	}
	return servers
}
