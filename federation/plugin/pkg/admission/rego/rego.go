/*
Copyright 2016 The Kubernetes Authors.

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

package rego

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"k8s.io/kubernetes/pkg/admission"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/runtime"
)

const (
	pluginName = "Rego"

	// TODO(tsandall): these ought to be configurable.
	opaBaseURL             = "http://opa.federation.svc.cluster.local:8181/v1"
	annotationDocumentPath = "/io/k8s/federation/annotations"
	opaName                = "opa"
)

type controller struct {
	*admission.Handler
	factory Factory
}

func (c *controller) Admit(a admission.Attributes) (err error) {

	// If the user is OPA (e.g., the enforcement component) then just return.
	// For now, OPA's identity is defined with a static token file.
	userInfo := a.GetUserInfo()
	if userInfo.GetName() == opaName {
		return nil
	}

	obj := a.GetObject()
	if obj == nil {
		return nil
	}

	// Encode object for API call to OPA.
	input, err := encodeObject(obj, a.GetKind().GroupVersion())
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	// Execute API call against OPA.
	result, err := c.factory.New(
		Query(annotationDocumentPath),
		Input(input)).
		Do()

	// If annotations document is not defined, then just stop.
	if err != nil {
		if _, ok := err.(Undefined); ok {
			return nil
		}
		return admission.NewForbidden(a, err)
	}

	// Otherwise, apply annotations to the object.
	annotations, err := decodeAnnotations(result)

	if err != nil {
		return admission.NewForbidden(a, err)
	}

	if len(annotations) == 0 {
		return nil
	}

	applyAnnotations(obj, annotations)

	return nil
}

func encodeObject(obj runtime.Object, gv unversioned.GroupVersion) (interface{}, error) {

	info, ok := api.Codecs.SerializerForMediaType("application/json", nil)
	if !ok {
		return nil, fmt.Errorf("serialization not supported")
	}

	codec := api.Codecs.EncoderForVersion(info.Serializer, gv)

	var buf bytes.Buffer

	if err := codec.Encode(obj, &buf); err != nil {
		return nil, err
	}

	var result interface{}

	if err := json.NewDecoder(&buf).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// decodeAnnotations reads the annotation response from the policy engine. If
// the annotation value is a non-string value, the value will be serialized to a
// string using JSON.
func decodeAnnotations(body interface{}) (map[string]string, error) {

	result, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result of type %T", body)
	}

	if len(result) == 0 {
		return nil, nil
	}

	annotations := map[string]string{}

	for key, annotation := range result {
		switch annotation := annotation.(type) {
		case string:
			annotations[key] = annotation
		default:
			bs, err := json.Marshal(annotation)
			if err != nil {
				return nil, err
			}
			annotations[key] = string(bs)
		}
	}

	return annotations, nil
}

func init() {
	admission.RegisterPlugin("Rego", func(client internalclientset.Interface, config io.Reader) (admission.Interface, error) {
		c := &controller{
			Handler: admission.NewHandler(admission.Create, admission.Update, admission.Delete, admission.Connect),
			factory: Factory{opaBaseURL},
		}
		return c, nil
	})
}
