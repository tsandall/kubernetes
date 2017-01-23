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
	"net/http"
	"strings"
)

type Undefined struct{}

func (u Undefined) Error() string {
	return "rejected by administrative policy"
}

type Factory struct {
	baseURL string
}

func (f Factory) New(options ...func(*Request)) *Request {
	r := NewRequest(f.baseURL)
	for i := range options {
		options[i](r)
	}
	return r
}

func Query(path string) func(*Request) {
	return func(r *Request) {
		r.queryPath = path
	}
}

func Input(input interface{}) func(*Request) {
	return func(r *Request) {
		r.input = input
	}
}

type Request struct {
	baseURL   string
	queryPath string
	input     interface{}
}

func NewRequest(baseURL string) *Request {
	return &Request{
		baseURL: baseURL,
	}
}

func (r *Request) Do() (interface{}, error) {

	if r.queryPath == "" {
		return nil, fmt.Errorf("not implemented")
	}

	if r.input == nil {
		return nil, fmt.Errorf("not implemented")
	}

	request := dataRequestV1{
		Input: r.input,
	}

	var buf bytes.Buffer

	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/data%s", r.baseURL, r.queryPath)
	resp, err := http.Post(url, "application/json", &buf)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, Undefined{}
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

type dataRequestV1 struct {
	Input interface{} `json:"input"`
}

type dataResponseV1 struct {
	Result interface{} `json:"result"`
}

type errorResponseV1 struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
