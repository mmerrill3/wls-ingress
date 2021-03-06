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

package collectors

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	operation = []string{"controller_namespace", "controller_class", "controller_pod"}
)

// Controller defines base metrics about the ingress controller
type Controller struct {
	prometheus.Collector
	cookieAdds     *prometheus.CounterVec
	constLabels    prometheus.Labels
	labels         prometheus.Labels
	leaderElection *prometheus.GaugeVec
}

// NewController creates a new prometheus collector for the
// Ingress controller operations
func NewController(pod, namespace, class string) *Controller {
	constLabels := prometheus.Labels{
		"controller_namespace": namespace,
		"controller_class":     class,
		"controller_pod":       pod,
	}

	cm := &Controller{
		constLabels: constLabels,

		labels: prometheus.Labels{
			"namespace": namespace,
			"class":     class,
		},
		cookieAdds: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: PrometheusNamespace,
				Name:      "cookie_adds",
				Help:      `Number of cookies added to redis`,
			},
			operation,
		),
		leaderElection: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   PrometheusNamespace,
				Name:        "leader_election_status",
				Help:        "Gauge reporting status of the leader election, 0 indicates follower, 1 indicates leader. 'name' is the string used to identify the lease",
				ConstLabels: constLabels,
			},
			[]string{"name"},
		),
	}

	return cm
}

// IncCheckCount increment the check counter
func (cm *Controller) IncCookieCount(namespace, name string) {
	labels := prometheus.Labels{
		"namespace": namespace,
		"ingress":   name,
	}
	cm.cookieAdds.MustCurryWith(cm.constLabels).With(labels).Inc()
}

// OnStartedLeading indicates the pod was elected as the leader
func (cm *Controller) OnStartedLeading(electionID string) {
	cm.leaderElection.WithLabelValues(electionID).Set(1.0)
}

// OnStoppedLeading indicates the pod stopped being the leader
func (cm *Controller) OnStoppedLeading(electionID string) {
	cm.leaderElection.WithLabelValues(electionID).Set(0)
}

// Describe implements prometheus.Collector
func (cm Controller) Describe(ch chan<- *prometheus.Desc) {
	cm.cookieAdds.Describe(ch)
}

// Collect implements the prometheus.Collector interface.
func (cm Controller) Collect(ch chan<- prometheus.Metric) {
	cm.cookieAdds.Collect(ch)
}
