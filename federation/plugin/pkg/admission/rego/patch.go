package rego

import "reflect"
import "strings"
import "k8s.io/kubernetes/pkg/runtime"

// applyAnnotations updates the object's metadata/annotations. If the object
// does not contain an annotations field, no change is performed.
func applyAnnotations(obj runtime.Object, annotations map[string]string) {
	val := reflect.Indirect(reflect.ValueOf(obj))
	annotationsFld, ok := getAnnotationsField(val)
	if !ok {
		return
	}

	orig := annotationsFld.Interface()
	if orig == nil {
		orig = map[string]string{}
	}

	origMap := orig.(map[string]string)

	for k := range origMap {
		if _, ok := annotations[k]; !ok {
			annotations[k] = origMap[k]
		}
	}

	annotationsFld.Set(reflect.ValueOf(annotations))
}

func getAnnotations(obj runtime.Object) map[string]string {
	val := reflect.Indirect(reflect.ValueOf(obj))
	annotationsFld, ok := getAnnotationsField(val)
	if !ok {
		return nil
	}

	orig := annotationsFld.Interface()
	if orig == nil {
		return nil
	}

	return orig.(map[string]string)
}

func getAnnotationsField(val reflect.Value) (reflect.Value, bool) {
	metadataFld, ok := getField(val, "metadata")
	if !ok {
		return reflect.Value{}, false
	}
	return getField(metadataFld, "annotations")
}

// getField returns the field identified by name. The name may refer to the JSON
// tag. If the field is not found, ok is false.
func getField(obj reflect.Value, field string) (val reflect.Value, ok bool) {

	tpe := obj.Type()

	if obj.Kind() == reflect.Ptr {
		obj = reflect.Indirect(obj)
		tpe = obj.Type()
	}

	val = obj.FieldByName(field)
	if val.IsValid() {
		return val, true
	}

	for i := 0; i < tpe.NumField(); i++ {
		fld := tpe.Field(i)
		for _, s := range strings.Split(fld.Tag.Get("json"), ",") {
			if s == field {
				return obj.FieldByName(fld.Name), true
			}
		}
	}

	return reflect.Zero(tpe), false
}
