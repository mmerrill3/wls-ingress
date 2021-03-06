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

package store

import (
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

// ConfigMapLister makes a Store that lists Configmaps.
type ConfigMapLister struct {
	cache.Store
}

// ByKey returns the ConfigMap matching key in the local ConfigMap Store.
func (cml *ConfigMapLister) ByKey(key string) (*apiv1.ConfigMap, error) {
	s, exists, err := cml.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, NotExistsError(key)
	}
	return s.(*apiv1.ConfigMap), nil
}
