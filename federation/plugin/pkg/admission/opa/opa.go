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

package opa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"k8s.io/kubernetes/pkg/admission"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/auth/user"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/runtime"
)

const (
	pluginName = "OPA"
)

type config struct {
	BaseURL         string   `json:"baseURL"`         // base URL of OPA API
	AnnotationsPath string   `json:"annotationsPath"` // path of annotation document to query
	IgnoreUserNames []string `json:"ignoreUserNames"` // list of names to ignore
}

// Ignored returns true if the request can be ignored based on the sender.
func (c config) Ignored(userInfo user.Info) bool {
	for _, x := range c.IgnoreUserNames {
		if x == userInfo.GetName() {
			return true
		}
	}
	return false
}

type controller struct {
	*admission.Handler
	config config
}

func (c *controller) Admit(a admission.Attributes) (err error) {

	if c.config.Ignored(a.GetUserInfo()) {
		return nil
	}

	obj := a.GetObject()
	if obj == nil {
		return nil
	}

	input, err := convertObject(obj, a.GetKind().GroupVersion())
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	result, err := newRequest(c.config.BaseURL, c.config.AnnotationsPath).
		WithInput(input).
		Do()

	if err != nil {
		// If annotations document is undefined then just ignore the request.
		// Otherwise, fail closed.
		if _, ok := err.(undefined); ok {
			return nil
		}
		return admission.NewForbidden(a, err)
	}

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

// convertObject returns the JSON representation of obj as a native Go value.
func convertObject(obj runtime.Object, gv unversioned.GroupVersion) (interface{}, error) {

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

// decodeAnnotations unmarshals the annotation response from the policy engine.
// If annotation values are not strings, they will be JSON serialized.
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

// undefined implements the error interface and indicates that the requested
// document was not found/is undefined.
type undefined struct{}

func (u undefined) Error() string {
	return "<undefined>"
}

// request provides a wrapper around the net/http package for performing Data
// API queries against OPA.
type request struct {
	baseURL string
	path    string
	input   *interface{}
}

func newRequest(baseURL string, path string) *request {
	return &request{
		baseURL: baseURL,
		path:    path,
	}
}

func (r *request) WithInput(input interface{}) *request {
	r.input = &input
	return r
}

// Do executes the request and returns the document identified by the path. If
// the document is undefined, the error will be undefined{}.
func (r *request) Do() (interface{}, error) {

	request := dataRequestV1{
		Input: r.input,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s%s", r.baseURL, r.path)
	resp, err := http.Post(url, "application/json", &buf)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, undefined{}
	}

	if resp.StatusCode == 200 {
		var response dataResponseV1
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, err
		}
		return response.Result, nil
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		var response errorResponseV1
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("code %v: %v", response.Code, response.Message)
	}

	return nil, fmt.Errorf("bad status code: %v", resp.StatusCode)
}

// dataRequestV1 defines the representation of the OPA Data API request message
// body.
type dataRequestV1 struct {
	Input *interface{} `json:"input,omitempty"`
}

// dataResponseV1 defines the representation of the OPA Data API response
// message body.
type dataResponseV1 struct {
	Result interface{} `json:"result"`
}

// errorResponseV1 defines the representation of the generic OPA API error.
type errorResponseV1 struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func init() {
	admission.RegisterPlugin(pluginName, func(client internalclientset.Interface, config io.Reader) (admission.Interface, error) {
		c := &controller{
			Handler: admission.NewHandler(admission.Create, admission.Update, admission.Delete, admission.Connect),
		}
		return c, nil
	})
}
