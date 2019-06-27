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

package annotations

import (
	"github.com/imdario/mergo"
	"k8s.io/klog"

	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/mmerrill3/wls-ingress/internal/ingress/annotations/proxy"
	"github.com/mmerrill3/wls-ingress/internal/ingress/errors"
	"github.com/mmerrill3/wls-ingress/internal/ingress/resolver"
	"github.com/mmerrill3/wls-ingress/internal/ingress/annotations/parser"
)

// DeniedKeyName name of the key that contains the reason to deny a location
const DeniedKeyName = "Denied"

// Ingress defines the valid annotations present in one WLS Ingress rule
type Ingress struct {
	metav1.ObjectMeta
	Proxy              proxy.Config
}

// Extractor defines the annotation parsers to be used in the extraction of annotations
type Extractor struct {
	annotations map[string]parser.IngressAnnotation
}

// NewAnnotationExtractor creates a new annotations extractor
func NewAnnotationExtractor(cfg resolver.Resolver) Extractor {
	return Extractor{
		map[string]parser.IngressAnnotation{
			"Proxy":                proxy.NewParser(cfg),
		},
	}
}

// Extract extracts the annotations from an Ingress
func (e Extractor) Extract(ing *networking.Ingress) *Ingress {
	pia := &Ingress{
		ObjectMeta: ing.ObjectMeta,
	}

	data := make(map[string]interface{})
	for name, annotationParser := range e.annotations {
		val, err := annotationParser.Parse(ing)
		klog.V(5).Infof("annotation %v in Ingress %v/%v: %v", name, ing.GetNamespace(), ing.GetName(), val)
		if err != nil {
			if errors.IsMissingAnnotations(err) {
				continue
			}

			if !errors.IsLocationDenied(err) {
				continue
			}

			_, alreadyDenied := data[DeniedKeyName]
			if !alreadyDenied {
				errString := err.Error()
				data[DeniedKeyName] = &errString
				klog.Errorf("error reading %v annotation in Ingress %v/%v: %v", name, ing.GetNamespace(), ing.GetName(), err)
				continue
			}

			klog.V(5).Infof("error reading %v annotation in Ingress %v/%v: %v", name, ing.GetNamespace(), ing.GetName(), err)
		}

		if val != nil {
			data[name] = val
		}
	}

	err := mergo.MapWithOverwrite(pia, data)
	if err != nil {
		klog.Errorf("unexpected error merging extracted annotations: %v", err)
	}

	return pia
}
