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
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eapache/channels"
	"github.com/mmerrill3/wls-ingress/internal/file"
	"github.com/mmerrill3/wls-ingress/internal/ingress/annotations/class"
	wls_config "github.com/mmerrill3/wls-ingress/internal/ingress/controller/config"
	"github.com/mmerrill3/wls-ingress/internal/ingress/controller/store"
	wls_handler "github.com/mmerrill3/wls-ingress/internal/ingress/handler/http"
	"github.com/mmerrill3/wls-ingress/internal/ingress/metric"
	"github.com/mmerrill3/wls-ingress/internal/ingress/status"
	"github.com/mmerrill3/wls-ingress/internal/k8s"
	"github.com/mmerrill3/wls-ingress/internal/redis"
	"github.com/mmerrill3/wls-ingress/internal/task"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
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
		updateCh:        channels.NewRingChannel(1024),
		stopLock:        &sync.Mutex{},
		fileSystem:      fs,
		runningConfig:   new(wls_config.Configuration),
		metricCollector: mc,
		redisManager: &redis.Manager{
			MasterName:    "mymaster",
			SentinelAddrs: []string{"redis-redis-ha.redis:26379"},
		},
		syncRateLimiter: flowcontrol.NewTokenBucketRateLimiter(config.SyncRateLimit, 1),
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

	n.syncQueue = task.NewTaskQueue(n.syncIngress)

	proxyMux := http.NewServeMux()
	handler, err := wls_handler.InstallHandler(proxyMux, n.runningConfig, n.redisManager)
	if err != nil {
		klog.Fatalf("could not configure the proxy http service - %v", err)
	}
	n.proxy = handler

	n.syncStatus = status.NewStatusSyncer(pod, status.Config{
		Client:                 config.Client,
		IngressLister:          n.store,
		EndpointLister:         n.store,
		RedisManager:           n.redisManager,
		UpdateStatusOnShutdown: true,
		PublishService:         config.PublishService,
	})

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

	syncQueue *task.Queue

	syncStatus status.Syncer

	// runningConfig contains the running configuration in the Backend
	runningConfig *wls_config.Configuration

	isShuttingDown bool

	store store.Storer

	syncRateLimiter flowcontrol.RateLimiter

	fileSystem filesystem.Filesystem

	metricCollector metric.Collector

	redisManager *redis.Manager

	currentLeader uint32

	// this is the main http listener that accepts requests before being proxied
	httpListener *http.Server

	idleConnsClosed chan struct{}

	proxy *wls_handler.WLSHandler
}

// Start starts a new NGINX master process running in the foreground.
func (n *WLSController) Start() {
	klog.Info("Starting WLS Ingress controller")

	go n.syncQueue.Run(time.Second, n.stopCh)
	// force initial sync
	n.syncQueue.EnqueueTask(task.GetDummyObject("initial-sync"))

	n.store.Run(n.stopCh)
	n.redisManager.Start()

	// we need to use the defined ingress class to allow multiple leaders
	// in order to update information about ingress status
	electionID := fmt.Sprintf("%v-%v", n.cfg.ElectionID, class.DefaultClass)
	if class.IngressClass != "" {
		electionID = fmt.Sprintf("%v-%v", n.cfg.ElectionID, class.IngressClass)
	}

	setupLeaderElection(&leaderElectionConfig{
		Client:     n.cfg.Client,
		ElectionID: electionID,
		OnStartedLeading: func(stopCh chan struct{}) {
			if n.syncStatus != nil {
				go n.syncStatus.Run(stopCh)
			}

			n.setLeader(true)
			n.metricCollector.OnStartedLeading(electionID)
		},
		OnStoppedLeading: func() {
			n.setLeader(false)
			n.metricCollector.OnStoppedLeading(electionID)
		},
		PodName:      n.podInfo.Name,
		PodNamespace: n.podInfo.Namespace,
		RedisManager: n.redisManager,
	})

	// start the http proxy service
	klog.Info("Starting WLS HTTP Proxy process")

	go func() {
		n.idleConnsClosed = make(chan struct{})
		n.httpListener = &http.Server{
			Addr:              fmt.Sprintf(":%v", 8080),
			Handler:           n.proxy,
			ReadTimeout:       180 * time.Second,
			ReadHeaderTimeout: 180 * time.Second,
			WriteTimeout:      180 * time.Second,
			IdleTimeout:       300 * time.Second,
		}
		if err := n.httpListener.ListenAndServe(); err != http.ErrServerClosed {
			// Error starting or closing listener:
			klog.Errorf("HTTP server ListenAndServe: %v", err)
		}
		<-n.idleConnsClosed
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
				n.syncQueue.EnqueueSkippableTask(evt.Obj)
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

	if n.syncStatus != nil {
		n.syncStatus.Shutdown()
	}

	klog.Info("Shutting down controller queues")
	close(n.stopCh)
	klog.Info("Shutting down redis manager")
	n.redisManager.Stop()
	klog.Info("Shutting down main http listener and idle connections")
	if err := n.httpListener.Shutdown(context.Background()); err != nil {
		// Error from closing listeners, or context timeout:
		klog.Errorf("HTTP server Shutdown: %v", err)
	}
	close(n.idleConnsClosed)
	return nil
}

func (n *WLSController) setLeader(leader bool) {
	var i uint32
	if leader {
		i = 1
	}
	atomic.StoreUint32(&n.currentLeader, i)
}

func (n *WLSController) isLeader() bool {
	return atomic.LoadUint32(&n.currentLeader) != 0
}

// OnUpdate is called by the synchronization loop whenever configuration
// changes were detected. The received backend Configuration is copied into the
// running configuration
func (n *WLSController) OnUpdate(ingressIn *wls_config.Configuration) error {
	n.runningConfig = ingressIn
	klog.V(4).Infof("Running config is now %+v", *n.runningConfig)
	n.proxy.UpdateConfiguration(ingressIn)
	//if I'm the leader, clean up the entries in redis immediately
	if n.isLeader() {
		var ips []string
		for _, service := range ingressIn.Services {
			for _, ep := range service.Backend.Endpoints {
				ips = append(ips, ep.Address)
			}
		}
		n.redisManager.Clean(ips)
	}
	return nil
}
