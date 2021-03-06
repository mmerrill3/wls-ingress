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

		publishSvc = flags.String("publish-service", "",
			`Service fronting the Ingress controller.
Takes the form "namespace/name". When used together with update-status, the
controller mirrors the address of this service's endpoints to the load-balancer
status of all Ingress objects it satisfies.`)

		watchNamespace = flags.String("watch-namespace", apiv1.NamespaceAll,
			`Namespace the controller watches for updates to Kubernetes objects.
This includes Ingresses, Services and all configuration resources. All
namespaces are watched if this parameter is left empty.`)

		defHealthzURL = flags.String("health-check-path", "/healthz",
			`URL path of the health check endpoint.
Configured inside the WLS Ingress status server. All requests received on the port
defined by the healthz-port parameter are forwarded internally to this path.`)

		defHealthCheckTimeout = flags.Int("health-check-timeout", 10, `Time limit, in seconds, for a probe to health-check-path to succeed.`)

		showVersion = flags.Bool("version", false,
			`Show release information about the WLS Ingress controller and exit.`)

		enableMetrics = flags.Bool("enable-metrics", true,
			`Enables the collection of WLS Ingress metrics`)

		electionID = flags.String("election-id", "wls-ingress-controller-leader",
			`Election id to use for Ingress status updates.`)

		syncRateLimit = flags.Float32("sync-rate-limit", 0.3,
			`Define the sync frequency upper limit`)

		httpPort             = flags.Int("http-port", 8080, `Port to use for servicing HTTP traffic.`)
		_                    = flags.Int("status-port", 18080, `Port to use for exposing WLS status pages.`)
		defServerPort        = flags.Int("default-server-port", 8181, `Port to use for exposing the default server (catch-all).`)
		healthzPort          = flags.Int("healthz-port", 10254, "Port to use for the healthz endpoint.")
		redisSentinelService = flags.String("redis-sentinel-service", "redis-redis-ha.redis", "the service name within the cluster for HA redis")
		redisSentinelPort    = flags.Int("redis-sentinel-port", 26379, "port where the HA redis service is running")
		redisMasterName      = flags.String("redis-master-name", "mymaster", "the name of the redis HA master")
		redisMaxRetries      = flags.Int("redis-max-retries", 3, "The limit on retrying the current master for a redi command")
		redisMinRetryBackoff = flags.Duration("redis-min-retry-backoff", 5*(time.Second), "The minimum time the redis client can wait before retrying the redis master")
		redisMaxRetryBackoff = flags.Duration("redis-max-retry-backoff", 10*(time.Second), "The maximum time the redis client can wait before retrying the redis master")
		redisNamespacePrefix = flags.String("redis-namespace-prefix", "WLS-ING", "the prefix for all keys that are stored in redis")
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
		PublishService: *publishSvc,
		ElectionID:     *electionID,
		SyncRateLimit:  *syncRateLimit,
		ListenPorts: &wls_config.ListenPorts{
			Default: *defServerPort,
			Health:  *healthzPort,
			HTTP:    *httpPort,
		},
		RedisSentinelService: *redisSentinelService,
		RedisSentinelPort:    *redisSentinelPort,
		RedisMasterName:      *redisMasterName,
		RedisMaxRetries:      *redisMaxRetries,
		RedisMinRetryBackoff: *redisMinRetryBackoff,
		RedisMaxRetryBackoff: *redisMaxRetryBackoff,
		RedisNamespacePrefix: *redisNamespacePrefix,
	}

	return false, config, nil
}
