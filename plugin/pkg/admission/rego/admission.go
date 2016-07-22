package rego

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/golang/glog"

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
	BaseURL     string `json:"baseURL" yaml:"baseURL"`
	AdmitDoc    string `json:"admitDoc" yaml:"admitDoc"`
	OverrideDoc string `json:"overrideDoc" yaml:"overrideDoc"`
}

type controller struct {
	*admission.Handler
	initialized bool
	initLock    sync.Mutex
	admitDoc    string
	overrideDoc string
	factory     factory
}

func (c *controller) Admit(a admission.Attributes) (err error) {

	if !c.initialized {
		c.initLock.Lock()
		defer c.initLock.Unlock()
		if !c.initialized {
			c.start()
			c.initialized = true
		}
	}

	client := c.factory.New()

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

	bs, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		panic(err)
	}
	glog.Infof("Checking OPA policies for: %v", string(bs))

	_, err = client.Query(c.admitDoc, map[string]interface{}{
		"request": req,
	})
	if err != nil {
		if _, ok := err.(undefined); ok {
			// TODO(tsandall): how to provide more informative error messages?
			return admission.NewForbidden(a, err)
		}
		return err
	}

	_, err = client.Query(c.overrideDoc, map[string]interface{}{
		"request": req,
	})
	if err != nil {
		if _, ok := err.(undefined); ok {
			return nil
		}
		return err
	}

	return nil
}

func (c *controller) start() {
	resourceTypes := []string{
		"pods",
		"nodes",
		"services",
		"replicationcontrollers",
	}
	client := c.factory.New()
	var wg sync.WaitGroup
	for i := range resourceTypes {
		wg.Add(1)
		resourceType := resourceTypes[i]
		go func() {
			initialized := false
			reflector, err := newReflector("http://localhost:8080/api/v1", resourceType, "")
			if err != nil {
				glog.Errorf("Failed start reflector: %v: %v", resourceType, err)
			}
			reflector.Start()
			for msg := range reflector.Rx {
				switch msg := msg.(type) {
				case *resyncObjects:
					if !initialized {
						if err := client.Patch(add, "/"+resourceType, map[string]interface{}{}); err != nil {
							glog.Errorf("Failed to initialize collection for %v: %v", resourceType, err)
						}
					}
					for _, obj := range msg.Items {
						uid := c.getUID(obj)
						if uid == "" {
							glog.Errorf("Failed to get UID for object: %v", obj)
							continue
						}
						path := fmt.Sprintf("/%v/%v", resourceType, uid)
						if err := client.Patch(add, path, obj); err != nil {
							glog.Errorf("Failed to handle resync/add for %v: %v", path, err)
							continue
						}
					}
					if !initialized {
						initialized = true
						wg.Done()
					}
				case *syncObject:
					uid := c.getUID(msg.Object)
					if uid == "" {
						glog.Errorf("Failed to get UID for object: %v", msg.Object)
						continue
					}
					path := fmt.Sprintf("/%v/%v", resourceType, uid)
					var op operation
					var obj interface{}
					switch msg.Type {
					case added:
						op = add
						obj = msg.Object
					case modified:
						op = replace
						obj = msg.Object
					case deleted:
						op = remove
					}
					if err := client.Patch(op, path, obj); err != nil {
						glog.Errorf("Failed to handle sync/%v for %v: %v", op, path, err)
						continue
					}
				}
			}
		}()
	}
	wg.Wait()
}

func (c *controller) getUID(obj interface{}) string {
	if obj, ok := obj.(map[string]interface{}); ok {
		if m, ok := obj["metadata"].(map[string]interface{}); ok {
			if u, ok := m["uid"].(string); ok {
				return u
			}
		}
	}
	return ""
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
		c := &controller{
			Handler:     admission.NewHandler(admission.Create, admission.Update, admission.Delete, admission.Connect),
			factory:     &httpClientFactory{cfg.BaseURL},
			admitDoc:    cfg.AdmitDoc,
			overrideDoc: cfg.OverrideDoc,
		}
		return c, nil
	})
}
