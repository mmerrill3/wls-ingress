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
	"bytes"
	"github.com/go-redis/redis"
	"k8s.io/klog"
	"strings"
	"sync"
	"time"
)

const (
	namespaceSeparator = ":"
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
	//synchronization mutex for cleaning
	mutx sync.Mutex

	RedisMaxRetries      int
	RedisMinRetryBackoff time.Duration
	RedisMaxRetryBackoff time.Duration

	// The namespace for the keys, default of WLS-ING:
	// If set, this is what will get prepended to all redis keys
	RedisNamespacePrefix string
}

//Clean cleans up the stale connections that are not in the list passed in.
func (r *Manager) Clean(endpoints []string) error {
	r.mutx.Lock()
	defer r.mutx.Unlock()
	klog.Infof("cleaning endpoints that are not in %v", endpoints)
	entries, err := r.getAllEntries()
	if err != nil {
		klog.Errorf("could not get clean up stale entries with list %v - %v", endpoints, err)
		return err
	}
	for k, v := range entries {
		if contains(endpoints, v) {
			klog.V(4).Infof("Skipping removal for entry for key %v with value %v", k, v)
			continue
		} else {
			klog.V(2).Infof("removing entry for key %v with value %v", k, v)
			cmd := r.client.Del(k)
			if cmd.Err() != nil {
				klog.Errorf("error deleting remote key - %v", cmd.Err())
			}
		}
	}
	return nil
}

//Start starts the redis communication
func (r *Manager) Start() error {
	r.mutx = sync.Mutex{}
	option := redis.FailoverOptions{
		MasterName:      r.MasterName,
		SentinelAddrs:   r.SentinelAddrs,
		MaxRetries:      r.RedisMaxRetries,
		MaxRetryBackoff: r.RedisMaxRetryBackoff,
		MinRetryBackoff: r.RedisMinRetryBackoff,
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
	cmd := r.client.Set(r.createNamespacedKey(jsession), endpoint, time.Hour)
	if cmd.Err() != nil {
		klog.Errorf("error setting remote value - %v", cmd.Err())
	}
	return nil
}

//RemoveEndpoint removes the cookie entries for the endpoint when its pod goes away
func (r *Manager) RemoveEndpoint(endpoint string) error {
	entrypMap, error := r.getAllEntries()
	if error != nil {
		klog.V(2).Info("No keys in redis")
		return nil
	}
	for cookie, upstream := range entrypMap {
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

//getAllEntries gets all the current entries
func (r *Manager) getAllEntries() (map[string]string, error) {
	entryMap := make(map[string]string)
	cmd := r.client.Keys(r.createNamespacedKey("*"))
	if cmd.Err() != nil {
		if cmd.Err() == redis.Nil {
			return nil, nil
		}
		klog.Errorf("error getting all remote keys - %v", cmd.Err())
		return nil, cmd.Err()
	}
	for _, cookie := range cmd.Val() {
		klog.V(8).Infof("Found cookie key in redis %v", cookie)
		endpoint, err := r.GetEndpoint(cookie, false)
		if err != nil {
			klog.Errorf("error getting remote value - %v", err)
			continue
		}

		entryMap[cookie] = endpoint
	}
	return entryMap, nil
}

//GetEndpoint returns the endpoint (IP Address) for the jsession cookie
func (r *Manager) GetEndpoint(jsession string, updateExpire bool) (string, error) {
	cmd := r.client.Get(r.createNamespacedKey(jsession))
	if cmd.Err() != nil {
		if cmd.Err() == redis.Nil {
			return "", nil
		} else {
			klog.Errorf("error getting remote value - %v", cmd.Err())
			return "", cmd.Err()
		}
	}
	klog.V(8).Infof("Found endpoint %v in redis for key %v", cmd.Val(), jsession)
	if updateExpire {
		expireCmd := r.client.Expire(r.createNamespacedKey(jsession), time.Hour)
		if expireCmd.Err() != nil {
			klog.Errorf("could not update the expiration on entry %v - %v", jsession, expireCmd.Err())
		}
	}
	return cmd.Val(), nil
}

//Stop ends the redis communication
func (r *Manager) Stop() error {
	r.client.Close()
	return nil
}

func (r *Manager) createNamespacedKey(key string) string {
	if strings.Contains(key, r.createNamespaced()) {
		return key
	}
	var retBuff bytes.Buffer
	retBuff.WriteString(r.createNamespaced())
	retBuff.WriteString(key)
	return retBuff.String()
}

func (r *Manager) createNamespaced() string {
	var retBuff bytes.Buffer
	retBuff.WriteString(r.RedisNamespacePrefix)
	retBuff.WriteString(namespaceSeparator)
	return retBuff.String()
}

// Contains tells whether a contains x.
func contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}
