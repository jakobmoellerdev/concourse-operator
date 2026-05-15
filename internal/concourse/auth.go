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

// Package concourse provides a thread-safe Concourse client cache and auth
// helpers for the concourse-operator controllers.
package concourse

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	concourcev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
)

// authRoundTripper injects an Authorization header on every request.
type authRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (a *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+a.token)
	return a.base.RoundTrip(r)
}

// basicAuthRoundTripper injects Basic auth on every request.
type basicAuthRoundTripper struct {
	base     http.RoundTripper
	username string
	password string
}

func (b *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.SetBasicAuth(b.username, b.password)
	return b.base.RoundTrip(r)
}

// BuildHTTPClient constructs an *http.Client from a ConcourseInstance spec,
// reading Secret values from the Kubernetes API via k8sClient.
func BuildHTTPClient(ctx context.Context, k8sClient client.Client, namespace string, spec concourcev1alpha1.ConcourseInstanceSpec) (*http.Client, error) {
	tlsCfg, err := buildTLSConfig(ctx, k8sClient, namespace, spec.TLS)
	if err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	}

	transport := &http.Transport{TLSClientConfig: tlsCfg}
	var base http.RoundTripper = transport

	switch {
	case spec.BasicAuth != nil:
		password, err := readSecretKey(ctx, k8sClient, namespace, spec.BasicAuth.PasswordRef)
		if err != nil {
			return nil, fmt.Errorf("read basic auth password: %w", err)
		}
		base = &basicAuthRoundTripper{base: transport, username: spec.BasicAuth.Username, password: password}

	case spec.TokenAuth != nil:
		token, err := readSecretKey(ctx, k8sClient, namespace, spec.TokenAuth.TokenRef)
		if err != nil {
			return nil, fmt.Errorf("read token auth: %w", err)
		}
		base = &authRoundTripper{base: transport, token: token}
	}

	return &http.Client{Transport: base}, nil
}

// NewConcourseClient creates a go-concourse Client from a built http.Client.
func NewConcourseClient(url string, httpClient *http.Client) goconcourse.Client {
	return goconcourse.NewClient(url, httpClient, false)
}

func buildTLSConfig(ctx context.Context, k8sClient client.Client, namespace string, cfg *concourcev1alpha1.TLSConfig) (*tls.Config, error) {
	tlsCfg := &tls.Config{} //nolint:gosec

	if cfg == nil {
		return tlsCfg, nil
	}

	tlsCfg.InsecureSkipVerify = cfg.InsecureSkipVerify //nolint:gosec

	if cfg.CARef != nil {
		caData, err := readSecretKey(ctx, k8sClient, namespace, *cfg.CARef)
		if err != nil {
			return nil, fmt.Errorf("read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caData)) {
			return nil, fmt.Errorf("failed to parse CA certificate from secret %s/%s", namespace, cfg.CARef.Name)
		}
		tlsCfg.RootCAs = pool
	}

	return tlsCfg, nil
}

func readSecretKey(ctx context.Context, k8sClient client.Client, namespace string, ref concourcev1alpha1.SecretKeySelector) (string, error) {
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, secret); err != nil {
		return "", fmt.Errorf("get secret %s/%s: %w", namespace, ref.Name, err)
	}
	val, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s/%s", ref.Key, namespace, ref.Name)
	}
	return string(val), nil
}
