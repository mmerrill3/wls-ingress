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

package redis

import (
	"github.com/go-redis/redis"
	"k8s.io/klog"
)

//Manager describes the interface into redis
type Manager struct {
	// go-redis client
	// connection pool with redis
	client *redis.Client
	// The master name.
	MasterName string
	// A seed list of host:port addresses of sentinel nodes.
	SentinelAddrs []string
}

//Start starts the redis communication
func (r *Manager) Start() error {
	option := redis.FailoverOptions{
		MasterName:    r.MasterName,
		SentinelAddrs: r.SentinelAddrs,
	}
	r.client = redis.NewFailoverClient(&option)
	klog.Infof("starting redis manager with master name %v and seed urls %v", r.MasterName, r.SentinelAddrs)
	if _, err := r.client.Ping().Result(); err != nil {
		klog.Errorf("could not create a connection to redis server: %v", err)
		return err
	}
	return nil
}

//AddCookie publishes the jsessionid cookie to redis for an endpoint
func (r *Manager) AddCookie(jsession, endpoint string) error {
	klog.V(2).Infof("adding entry for key %v with value %v", jsession, endpoint)
	cmd := r.client.Set(jsession, endpoint, 0)
	if cmd.Err() != nil {
		klog.Errorf("error setting remote value - %v", cmd.Err())
	}
	return nil
}

//RemoveEndpoint removes the cookie entries for the endpoint when its pod goes away
func (r *Manager) RemoveEndpoint(endpoint string) error {
	cmd := r.client.Keys("*")
	if cmd.Err() != nil {
		if cmd.Err() == redis.Nil {
			return nil
		}
		klog.Errorf("error getting all remote keys - %v", cmd.Err())
		return cmd.Err()
	}
	for _, cookie := range cmd.Val() {
		upstream, err := r.GetEndpoint(cookie)
		if err != nil {
			klog.Errorf("error getting remote value - %v", err)
		}
		if endpoint == upstream {
			klog.V(2).Infof("removing entry for key %v with value %v", cookie, upstream)
			cmd := r.client.Del(cookie)
			if cmd.Err() != nil {
				klog.Errorf("error deleting remote key - %v", cmd.Err())
			}
		}
	}
	return nil
}

//GetEndpoint returns the endpoint (IP Address) for the jsession cookie
func (r *Manager) GetEndpoint(jsession string) (string, error) {
	cmd := r.client.Get(jsession)
	if cmd.Err() != nil {
		if cmd.Err() == redis.Nil {
			return "", nil
		} else {
			klog.Errorf("error getting remote value - %v", cmd.Err())
			return "", cmd.Err()
		}
	}
	return cmd.Val(), nil
}

//Stop ends the redis communication
func (r *Manager) Stop() error {
	r.client.Close()
	return nil
}
