/*
Copyright 2026.

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

//go:build integration

package integration_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	concourseURL  = "http://localhost:8080"
	concourseUser = "test"
	concoursePass = "test"
)

var concourseClient goconcourse.Client

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Concourse Integration Suite")
}

var _ = BeforeSuite(func() {
	token, err := getOAuthToken()
	Expect(err).NotTo(HaveOccurred(), "failed to get OAuth token from Concourse — is it running at %s?", concourseURL)

	transport := &bearerTransport{base: http.DefaultTransport, token: token}
	concourseClient = goconcourse.NewClient(concourseURL, &http.Client{Transport: transport}, false)
})

// getOAuthToken fetches a bearer token using the local user password grant.
func getOAuthToken() (string, error) {
	body := strings.NewReader("grant_type=password&username=" + concourseUser + "&password=" + concoursePass +
		"&scope=openid+profile+email+federated:id+groups")
	req, err := http.NewRequest(http.MethodPost, concourseURL+"/sky/issuer/token", body)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth("fly", "Zmx5") // fly client credentials (well-known defaults)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, data)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}
	return result.AccessToken, nil
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (b *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}
