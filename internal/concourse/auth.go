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
	"golang.org/x/oauth2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	concourcev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
)

// flyOAuthClientID and flyOAuthClientSecret are the well-known credentials
// Concourse's sky/issuer endpoint accepts for the password grant flow.
const (
	flyOAuthClientID     = "fly"
	flyOAuthClientSecret = "Zmx5"
)

// passwordTokenSource re-runs the OAuth2 password grant each time the cached
// token is expired. Concourse's dex does not issue refresh tokens for this
// grant type, so re-authentication is the only way to renew.
type passwordTokenSource struct {
	oauthCfg  *oauth2.Config
	transport http.RoundTripper
	username  string
	password  string
}

func (s *passwordTokenSource) Token() (*oauth2.Token, error) {
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: s.transport})
	token, err := s.oauthCfg.PasswordCredentialsToken(ctx, s.username, s.password) //nolint:staticcheck
	if err != nil {
		return nil, fmt.Errorf("oauth2 password grant: %w", err)
	}
	return token, nil
}

// BuildHTTPClient constructs an *http.Client from a Instance spec,
// reading Secret values from the Kubernetes API via k8sClient.
func BuildHTTPClient(ctx context.Context, k8sClient client.Client, namespace string, spec concourcev1alpha1.InstanceSpec) (*http.Client, error) {
	tlsCfg, err := buildTLSConfig(ctx, k8sClient, namespace, spec.TLS)
	if err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	}

	transport := &http.Transport{TLSClientConfig: tlsCfg}

	switch {
	case spec.BasicAuth != nil:
		password, err := readSecretKey(ctx, k8sClient, namespace, spec.BasicAuth.PasswordRef)
		if err != nil {
			return nil, fmt.Errorf("read basic auth password: %w", err)
		}
		oauthCfg := &oauth2.Config{
			ClientID:     flyOAuthClientID,
			ClientSecret: flyOAuthClientSecret,
			Endpoint:     oauth2.Endpoint{TokenURL: spec.URL + "/sky/issuer/token"},
			Scopes:       []string{"openid", "profile", "email", "federated:id", "groups"},
		}
		src := &passwordTokenSource{
			oauthCfg:  oauthCfg,
			transport: transport,
			username:  spec.BasicAuth.Username,
			password:  password,
		}
		// ReuseTokenSource caches the token in memory and only calls src.Token()
		// again once the token has expired, making it safe for concurrent use.
		return &http.Client{
			Transport: &oauth2.Transport{
				Source: oauth2.ReuseTokenSource(nil, src),
				Base:   transport,
			},
		}, nil

	case spec.TokenAuth != nil:
		token, err := readSecretKey(ctx, k8sClient, namespace, spec.TokenAuth.TokenRef)
		if err != nil {
			return nil, fmt.Errorf("read token auth: %w", err)
		}
		return &http.Client{
			Transport: &oauth2.Transport{
				Source: oauth2.StaticTokenSource(&oauth2.Token{
					AccessToken: token,
					TokenType:   "bearer",
				}),
				Base: transport,
			},
		}, nil
	}

	return &http.Client{Transport: transport}, nil
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
