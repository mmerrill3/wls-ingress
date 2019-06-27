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

package controller

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/eapache/channels"
	"github.com/mmerrill3/wls-ingress/internal/file"
	"github.com/mmerrill3/wls-ingress/internal/ingress"
	"github.com/mmerrill3/wls-ingress/internal/ingress/controller/store"
	wls_handler "github.com/mmerrill3/wls-ingress/internal/ingress/handler/http"
	"github.com/mmerrill3/wls-ingress/internal/ingress/metric"
	"github.com/mmerrill3/wls-ingress/internal/k8s"
	"github.com/mmerrill3/wls-ingress/internal/redis"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/filesystem"
)

// NewWLSController creates a new WLS Ingress controller.
func NewWLSController(config *Configuration, mc metric.Collector, fs file.Filesystem) *WLSController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&clientv1.EventSinkImpl{
		Interface: config.Client.CoreV1().Events(config.Namespace),
	})

	n := &WLSController{
		cfg:             config,
		stopCh:          make(chan struct{}),
		stopLock:        &sync.Mutex{},
		fileSystem:      fs,
		runningConfig:   new(ingress.Configuration),
		metricCollector: mc,
		redisManager: &redis.Manager{
			MasterName:    "mymaster",
			SentinelAddrs: []string{"redis-redis-ha:26379"},
		},
	}

	pod, err := k8s.GetPodDetails(config.Client)
	if err != nil {
		klog.Fatalf("unexpected error obtaining pod information: %v", err)
	}
	n.podInfo = pod

	n.store = store.New(
		config.Namespace,
		config.Client,
		fs,
		n.updateCh,
		pod)

	proxyMux := http.NewServeMux()
	handler, err := wls_handler.InstallHandler(proxyMux)
	if err != nil {
		klog.Fatalf("could not configure the proxy http service - %v", err)
	}
	n.proxy = handler
	return n
}

// WLSController describes a WLS Ingress controller.
type WLSController struct {
	podInfo *k8s.PodInfo

	cfg *Configuration

	recorder record.EventRecorder

	// stopLock is used to enforce that only a single call to Stop send at
	// a given time. We allow stopping through an HTTP endpoint and
	// allowing concurrent stoppers leads to stack traces.
	stopLock *sync.Mutex

	stopCh   chan struct{}
	updateCh *channels.RingChannel

	// errCh is used to detect errors with the HTTP processe
	errCh chan error

	// runningConfig contains the running configuration in the Backend
	runningConfig *ingress.Configuration

	isShuttingDown bool

	store store.Storer

	fileSystem filesystem.Filesystem

	metricCollector metric.Collector

	redisManager *redis.Manager

	proxy *wls_handler.WLSHandler
}

// Start starts a new NGINX master process running in the foreground.
func (n *WLSController) Start() {
	klog.Info("Starting WLS Ingress controller")

	n.store.Run(n.stopCh)

	n.redisManager.Start()

	// start the http proxy service
	klog.Info("Starting WLS HTTP Proxy process")

	go func() {
		server := &http.Server{
			Addr:              fmt.Sprintf(":%v", 80),
			Handler:           n.proxy,
			ReadTimeout:       180 * time.Second,
			ReadHeaderTimeout: 180 * time.Second,
			WriteTimeout:      180 * time.Second,
			IdleTimeout:       300 * time.Second,
		}
		klog.Fatal(server.ListenAndServe())
	}()

	for {
		select {
		case err := <-n.errCh:
			if n.isShuttingDown {
				break
			}
			klog.Errorf("Received an error on the error channel - %v", err)
		case event := <-n.updateCh.Out():
			if n.isShuttingDown {
				break
			}
			if evt, ok := event.(store.Event); ok {
				klog.V(3).Infof("Event %v received - object %v", evt.Type, evt.Obj)
				//figure out if we need to send an update to redis
			} else {
				klog.Warningf("Unexpected event type received %T", event)
			}
		case <-n.stopCh:
			break
		}
	}
}

// Stop gracefully stops the WLS HTTP proxy.
func (n *WLSController) Stop() error {
	n.isShuttingDown = true

	n.stopLock.Lock()
	defer n.stopLock.Unlock()

	klog.Info("Shutting down controller queues")
	close(n.stopCh)
	n.redisManager.Stop()
	return nil
}
