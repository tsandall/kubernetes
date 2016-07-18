package rego

import (
	"testing"

	"k8s.io/kubernetes/pkg/admission"
	"k8s.io/kubernetes/pkg/api"
)

type mockFactory struct{}

func (f *mockFactory) New() client {
	return &mockClient{}
}

type mockClient struct{}

func (c *mockClient) Query(doc string, globals map[string]interface{}) (interface{}, error) {
	return nil, nil
}

func TestAdmit(t *testing.T) {
	c := controller{
		factory: &mockFactory{},
	}
	version := "v0"
	kind := api.Kind("Pod").WithVersion(version)
	resource := "pods"
	namespace := "testns"
	name := "testname"
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: name,
		},
		Spec: api.PodSpec{},
	}
	record := admission.NewAttributesRecord(pod, nil, kind, namespace, name, api.Resource(resource).WithVersion(version), "", admission.Create, nil)
	err := c.Admit(record)
	if err != nil {
		t.Errorf("Expected success but got: %v", err)
	}
}
