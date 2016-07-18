package rego

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type factory interface {
	New() client
}

type client interface {
	Query(doc string, globals map[string]interface{}) (interface{}, error)
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
