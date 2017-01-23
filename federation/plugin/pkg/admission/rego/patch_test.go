package rego

import "testing"
import "reflect"
import "encoding/json"
import "k8s.io/kubernetes/pkg/api"

func TestApplyAnnotations(t *testing.T) {

	tests := []struct {
		input       string
		annotations string
		expected    string
	}{
		{`{}`, `{"foo": "bar"}`, `{"foo": "bar"}`},
		{`{"metadata": {}}`, `{"foo": "bar"}`, `{"foo": "bar"}`},
		{`{"metadata": {"annotations": {}}}`, `{"foo": "bar"}`, `{"foo": "bar"}`},
		{`{"metadata": {"annotations": {"foo": "baz"}}}`, `{"foo": "bar"}`, `{"foo": "bar"}`},
		{`{"metadata": {"annotations": {"baz": "qux"}}}`, `{"foo": "bar"}`, `{"baz": "qux", "foo": "bar"}`},
	}

	for _, tc := range tests {
		var pod api.Pod

		if err := json.Unmarshal([]byte(tc.input), &pod); err != nil {
			panic(err)
		}

		annotations := map[string]string{}

		if err := json.Unmarshal([]byte(tc.annotations), &annotations); err != nil {
			panic(err)
		}

		expected := map[string]string{}

		if err := json.Unmarshal([]byte(tc.expected), &expected); err != nil {
			panic(err)
		}

		applyAnnotations(&pod, annotations)

		if !reflect.DeepEqual(pod.ObjectMeta.Annotations, expected) {
			t.Errorf("Expected annotations to equal %v but got (whole pod): %v", expected, pod)
		}
	}

}
