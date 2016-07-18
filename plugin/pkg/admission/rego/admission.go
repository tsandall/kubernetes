package rego

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"k8s.io/kubernetes/pkg/admission"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/auth/user"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/util/yaml"
)

const (
	pluginName = "Rego"
)

type request struct {
	Kind        unversioned.GroupVersionKind     `json:"kind"`
	Name        string                           `json:"name"`
	Namespace   string                           `json:"namespace"`
	Resource    unversioned.GroupVersionResource `json:"resource"`
	SubResource string                           `json:"subResource"`
	Object      runtime.Object                   `json:"object"`
	OldObject   runtime.Object                   `json:"oldObject"`
	Operation   admission.Operation              `json:"operation"`
	UserInfo    user.Info                        `json:"userInfo"`
}

type controllerConfig struct {
	BaseURL  string `json:"baseURL" yaml:"baseURL"`
	AdmitDoc string `json:"admitDoc" yaml:"admitDoc"`
}

type controller struct {
	*admission.Handler
	admitDoc string
	factory  factory
}

func (c *controller) Admit(a admission.Attributes) (err error) {

	req := &request{
		Kind:        a.GetKind(),
		Name:        a.GetName(),
		Namespace:   a.GetNamespace(),
		Resource:    a.GetResource(),
		SubResource: a.GetSubresource(),
		Object:      a.GetObject(),
		OldObject:   a.GetOldObject(),
		Operation:   a.GetOperation(),
		UserInfo:    a.GetUserInfo(),
	}

	client := c.factory.New()

	_, err = client.Query(c.admitDoc, map[string]interface{}{
		"request": req,
	})

	if err != nil {
		if _, ok := err.(undefined); ok {
			return admission.NewForbidden(a, err)
		}
		return err
	}

	return nil
}

func init() {
	admission.RegisterPlugin("Rego", func(client internalclientset.Interface, config io.Reader) (admission.Interface, error) {
		if config == nil {
			return nil, fmt.Errorf("config required for %v Admission Controller", pluginName)
		}
		cfg := controllerConfig{}
		data, err := ioutil.ReadAll(config)
		if err != nil {
			return nil, err
		}
		jsonData, err := yaml.ToJSON(data)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(jsonData, &cfg); err != nil {
			return nil, err
		}
		return &controller{
			Handler:  admission.NewHandler(admission.Create, admission.Update),
			factory:  &httpClientFactory{cfg.BaseURL},
			admitDoc: cfg.AdmitDoc,
		}, nil
	})
}
