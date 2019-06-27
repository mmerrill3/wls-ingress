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

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/spf13/pflag"

	"github.com/mmerrill3/wls-ingress/internal/ingress/annotations/class"
	"github.com/mmerrill3/wls-ingress/internal/ingress/controller"
	wls_config "github.com/mmerrill3/wls-ingress/internal/ingress/controller/config"
	ing_net "github.com/mmerrill3/wls-ingress/internal/net"
	"github.com/mmerrill3/wls-ingress/internal/wls"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

func parseFlags() (bool, *controller.Configuration, error) {
	var (
		flags = pflag.NewFlagSet("", pflag.ExitOnError)

		apiserverHost = flags.String("apiserver-host", "",
			`Address of the Kubernetes API server.
Takes the form "protocol://address:port". If not specified, it is assumed the
program runs inside a Kubernetes cluster and local discovery is attempted.`)

		kubeConfigFile = flags.String("kubeconfig", "",
			`Path to a kubeconfig file containing authorization and API server information.`)

		ingressClass = flags.String("ingress-class", "",
			`Name of the ingress class this controller satisfies.
The class of an Ingress object is set using the annotation "kubernetes.io/ingress.class".
All ingress classes are satisfied if this parameter is left empty.`)

		watchNamespace = flags.String("watch-namespace", apiv1.NamespaceAll,
			`Namespace the controller watches for updates to Kubernetes objects.
This includes Ingresses, Services and all configuration resources. All
namespaces are watched if this parameter is left empty.`)

		defHealthzURL = flags.String("health-check-path", "/healthz",
			`URL path of the health check endpoint.
Configured inside the NGINX status server. All requests received on the port
defined by the healthz-port parameter are forwarded internally to this path.`)

		defHealthCheckTimeout = flags.Int("health-check-timeout", 10, `Time limit, in seconds, for a probe to health-check-path to succeed.`)

		showVersion = flags.Bool("version", false,
			`Show release information about the WLS Ingress controller and exit.`)

		enableMetrics = flags.Bool("enable-metrics", true,
			`Enables the collection of NGINX metrics`)
		httpPort      = flags.Int("http-port", 80, `Port to use for servicing HTTP traffic.`)
		_             = flags.Int("status-port", 18080, `Port to use for exposing WLS status pages.`)
		defServerPort = flags.Int("default-server-port", 8181, `Port to use for exposing the default server (catch-all).`)
		healthzPort   = flags.Int("healthz-port", 10254, "Port to use for the healthz endpoint.")
	)

	flags.MarkDeprecated("status-port", `The status port is a unix socket now.`)

	flag.Set("logtostderr", "true")

	flags.AddGoFlagSet(flag.CommandLine)
	flags.Parse(os.Args)

	// Workaround for this issue:
	// https://github.com/kubernetes/kubernetes/issues/17162
	flag.CommandLine.Parse([]string{})

	pflag.VisitAll(func(flag *pflag.Flag) {
		klog.V(2).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
	})

	if *showVersion {
		return true, nil, nil
	}

	if *ingressClass != "" {
		klog.Infof("Watching for Ingress class: %s", *ingressClass)

		if *ingressClass != class.DefaultClass {
			klog.Warningf("Only Ingresses with class %q will be processed by this Ingress controller", *ingressClass)
		}

		class.IngressClass = *ingressClass
	}

	// check port collisions
	if !ing_net.IsPortAvailable(*httpPort) {
		return false, nil, fmt.Errorf("Port %v is already in use. Please check the flag --http-port", *httpPort)
	}

	if !ing_net.IsPortAvailable(*defServerPort) {
		return false, nil, fmt.Errorf("Port %v is already in use. Please check the flag --default-server-port", *defServerPort)
	}

	wls.HealthPath = *defHealthzURL

	if *defHealthCheckTimeout > 0 {
		wls.HealthCheckTimeout = time.Duration(*defHealthCheckTimeout) * time.Second
	}

	config := &controller.Configuration{
		APIServerHost:  *apiserverHost,
		KubeConfigFile: *kubeConfigFile,
		EnableMetrics:  *enableMetrics,
		Namespace:      *watchNamespace,
		ListenPorts: &wls_config.ListenPorts{
			Default: *defServerPort,
			Health:  *healthzPort,
			HTTP:    *httpPort,
		},
	}

	return false, config, nil
}
