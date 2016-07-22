package rego

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/golang/glog"
)

type factory interface {
	New() client
}

type operation string

const (
	add     operation = "add"
	remove  operation = "remove"
	replace operation = "replace"
)

type client interface {
	Query(doc string, globals map[string]interface{}) (interface{}, error)
	Patch(op operation, path string, obj interface{}) error
}

type undefined struct{}

func (u undefined) Error() string {
	return "rejected by administrative policy"
}

type httpClientFactory struct {
	baseURL string
}

func (f *httpClientFactory) New() client {
	return &httpClient{
		baseURL: f.baseURL,
	}
}

type httpClient struct {
	baseURL string
}

func (c *httpClient) Query(doc string, globals map[string]interface{}) (interface{}, error) {

	params := []string{}
	for k, v := range globals {
		bs, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		params = append(params, fmt.Sprintf("global=%s:%s", k, url.QueryEscape(string(bs))))
	}

	url := fmt.Sprintf("%s/data%s?%s", c.baseURL, doc, strings.Join(params, "&"))
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	d := json.NewDecoder(resp.Body)
	var v interface{}
	if err := d.Decode(&v); err != nil {
		return nil, err
	}

	if resp.StatusCode == 200 {
		return v, nil
	}

	if resp.StatusCode == 404 {
		if v, ok := v.(map[string]interface{}); ok {
			if v["IsUndefined"] != nil {
				return nil, undefined{}
			}
		}
	}

	return nil, fmt.Errorf("bad response: %v", resp.StatusCode)
}

func (c *httpClient) Patch(op operation, path string, obj interface{}) error {

	patch := map[string]interface{}{
		"path": "/",
		"op":   op,
	}

	if obj != nil {
		patch["value"] = obj
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.Encode([]interface{}{
		patch,
	})

	client := http.DefaultClient

	p := fmt.Sprintf("%s/data%s", c.baseURL, path)
	req, err := http.NewRequest("PATCH", p, &buf)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	glog.Infof("Patch %v %v: response: %v", op, p, resp.StatusCode)

	if resp.StatusCode != 204 {
		decoder := json.NewDecoder(resp.Body)
		body := map[string]interface{}{}
		msg := fmt.Sprintf("patch failed (code: %v)", resp.StatusCode)
		if err := decoder.Decode(&body); err == nil {
			msg = fmt.Sprintf("%s: %s", msg, body["Message"])
		}
		return fmt.Errorf(msg)
	}

	return nil
}
