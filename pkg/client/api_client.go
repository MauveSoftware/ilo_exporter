// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"golang.org/x/sync/semaphore"

	"github.com/sirupsen/logrus"
)

// APIClient communicates with the API and parses the results
type APIClient struct {
	url      string
	hostName string
	username string
	password string
	client   *http.Client
	debug    bool
	sem      *semaphore.Weighted
}

// ClientOption applies options to APIClient
type ClientOption func(*APIClient)

// WithInsecure disables TLS certificate validation
func WithInsecure() ClientOption {
	return func(c *APIClient) {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		c.client = &http.Client{Transport: tr}
	}
}

// WithDebug enables debug mode
func WithDebug() ClientOption {
	return func(c *APIClient) {
		c.debug = true
	}
}

// WithMaxConcurrentRequests defines the maximum number of GET requests sent against API concurrently
func WithMaxConcurrentRequests(max uint) ClientOption {
	return func(c *APIClient) {
		c.sem = semaphore.NewWeighted(int64(max))
	}
}

// NewClient creates a new client instance
func NewClient(hostName, username, password string, opts ...ClientOption) Client {
	cl := &APIClient{
		url:      fmt.Sprintf("https://%s/rest/v1/", hostName),
		hostName: hostName,
		username: username,
		password: password,
		client:   &http.Client{},
		sem:      semaphore.NewWeighted(1),
	}

	for _, o := range opts {
		o(cl)
	}

	return cl
}

// HostName returns the name of the host
func (cl *APIClient) HostName() string {
	return cl.hostName
}

// Get retrieves the ressource from the API and unmashals the json retrieved
func (cl *APIClient) Get(path string, obj interface{}) error {
	b, err := cl.get(path)
	if err != nil {
		return err
	}

	err = json.Unmarshal(b, obj)
	return err
}

func (cl *APIClient) get(path string) ([]byte, error) {
	cl.sem.Acquire(context.Background(), 1)
	defer cl.sem.Release(1)

	uri := strings.Trim(cl.url, "/") + "/" + strings.Trim(path, "/")

	logrus.Infof("GET %s", uri)

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(cl.username, cl.password)
	req.Header.Add("Content-Type", "application/json")

	resp, err := cl.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf(resp.Status)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if cl.debug {
		logrus.Infof("Status Code: %s", resp.Status)
		logrus.Infof("Response: %s", string(b))
	}

	return b, err
}
