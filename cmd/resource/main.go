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

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Source represents the Concourse resource source configuration.
type Source struct {
	Kind       string `json:"kind"`                 // "ConfigMap" or "Secret" (default: "ConfigMap")
	Name       string `json:"name"`                 // Name of the ConfigMap or Secret
	Namespace  string `json:"namespace"`            // Namespace (default: current namespace)
	Kubeconfig string `json:"kubeconfig,omitempty"` // Optional raw kubeconfig string for testing/out-of-cluster
}

// Version represents the version format in Concourse.
type Version struct {
	Hash string `json:"hash"`
}

// CheckRequest is the stdin payload for /opt/resource/check.
type CheckRequest struct {
	Source  Source   `json:"source"`
	Version *Version `json:"version,omitempty"`
}

// InRequest is the stdin payload for /opt/resource/in.
type InRequest struct {
	Source  Source  `json:"source"`
	Version Version `json:"version"`
}

// MetadataEntry represents an entry in Concourse resource metadata.
type MetadataEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// InResponse is the stdout payload for /opt/resource/in.
type InResponse struct {
	Version  Version         `json:"version"`
	Metadata []MetadataEntry `json:"metadata,omitempty"`
}

// OutResponse is the stdout payload for /opt/resource/out.
type OutResponse struct {
	Version  Version         `json:"version"`
	Metadata []MetadataEntry `json:"metadata,omitempty"`
}

const (
	actionCheck = "check"
	actionIn    = "in"
	actionOut   = "out"
)

func main() {
	if err := run(os.Args, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	action := actionCheck
	if len(args) > 0 {
		base := filepath.Base(args[0])
		switch base {
		case actionCheck, actionIn, actionOut:
			action = base
		default:
			if len(args) > 1 {
				action = args[1]
			}
		}
	}

	switch action {
	case actionCheck:
		return handleCheck(stdin, stdout)
	case actionIn:
		destDir := "."
		if len(args) > 1 && filepath.Base(args[0]) == actionIn {
			destDir = args[1]
		} else if len(args) > 2 {
			destDir = args[2]
		}
		return handleIn(stdin, stdout, destDir)
	case actionOut:
		return handleOut(stdin, stdout)
	default:
		return fmt.Errorf("unknown action: %q", action)
	}
}

func getKubernetesClient(src Source) (kubernetes.Interface, string, error) {
	var cfg *rest.Config
	var defaultNamespace string
	var err error

	if src.Kubeconfig != "" {
		clientConfig, err := clientcmd.NewClientConfigFromBytes([]byte(src.Kubeconfig))
		if err != nil {
			return nil, "", fmt.Errorf("parse kubeconfig from source: %w", err)
		}
		cfg, err = clientConfig.ClientConfig()
		if err != nil {
			return nil, "", fmt.Errorf("get rest config: %w", err)
		}
		defaultNamespace, _, _ = clientConfig.Namespace()
	} else if kubeconfigPath := os.Getenv("KUBECONFIG"); kubeconfigPath != "" {
		clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
			&clientcmd.ConfigOverrides{},
		)
		cfg, err = clientConfig.ClientConfig()
		if err != nil {
			return nil, "", fmt.Errorf("load KUBECONFIG: %w", err)
		}
		defaultNamespace, _, _ = clientConfig.Namespace()
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, "", fmt.Errorf("build in-cluster config: %w", err)
		}
		if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			defaultNamespace = strings.TrimSpace(string(data))
		}
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("create kubernetes client: %w", err)
	}

	ns := src.Namespace
	if ns == "" {
		ns = defaultNamespace
	}
	if ns == "" {
		ns = "default"
	}

	return clientset, ns, nil
}

func fetchData(ctx context.Context, client kubernetes.Interface, src Source, ns string) (map[string][]byte, error) {
	kind := strings.ToLower(src.Kind)
	if kind == "" || kind == "configmap" || kind == "cm" {
		cm, err := client.CoreV1().ConfigMaps(ns).Get(ctx, src.Name, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil, fmt.Errorf("configMap %q not found in namespace %q", src.Name, ns)
			}
			return nil, fmt.Errorf("get configMap %q in namespace %q: %w", src.Name, ns, err)
		}
		result := make(map[string][]byte, len(cm.Data)+len(cm.BinaryData))
		for k, v := range cm.Data {
			result[k] = []byte(v)
		}
		maps.Copy(result, cm.BinaryData)
		return result, nil
	}

	if kind == "secret" || kind == "secrets" {
		sec, err := client.CoreV1().Secrets(ns).Get(ctx, src.Name, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil, fmt.Errorf("secret %q not found in namespace %q", src.Name, ns)
			}
			return nil, fmt.Errorf("get secret %q in namespace %q: %w", src.Name, ns, err)
		}
		result := make(map[string][]byte, len(sec.Data)+len(sec.StringData))
		maps.Copy(result, sec.Data)
		for k, v := range sec.StringData {
			result[k] = []byte(v)
		}
		return result, nil
	}

	return nil, fmt.Errorf("unsupported kind: %q (expected ConfigMap or Secret)", src.Kind)
}

func computeHash(data map[string][]byte) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write(data[k])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func handleCheck(stdin io.Reader, stdout io.Writer) error {
	var req CheckRequest
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		return fmt.Errorf("decode check request: %w", err)
	}

	client, ns, err := getKubernetesClient(req.Source)
	if err != nil {
		return err
	}

	data, err := fetchData(context.Background(), client, req.Source, ns)
	if err != nil {
		return err
	}

	currentHash := computeHash(data)
	versions := []Version{}

	if req.Version == nil || req.Version.Hash == "" {
		versions = append(versions, Version{Hash: currentHash})
	} else if req.Version.Hash != currentHash {
		versions = append(versions, Version{Hash: currentHash})
	} else {
		versions = append(versions, Version{Hash: req.Version.Hash})
	}

	return json.NewEncoder(stdout).Encode(versions)
}

func handleIn(stdin io.Reader, stdout io.Writer, destDir string) error {
	var req InRequest
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		return fmt.Errorf("decode in request: %w", err)
	}

	client, ns, err := getKubernetesClient(req.Source)
	if err != nil {
		return err
	}

	data, err := fetchData(context.Background(), client, req.Source, ns)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create destination directory %q: %w", destDir, err)
	}

	metadata := make([]MetadataEntry, 0, 4)
	metadata = append(metadata,
		MetadataEntry{Name: "kind", Value: req.Source.Kind},
		MetadataEntry{Name: "name", Value: req.Source.Name},
		MetadataEntry{Name: "namespace", Value: ns},
	)

	var keyList []string
	for filename, content := range data {
		filePath := filepath.Join(destDir, filename)
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			return fmt.Errorf("write file %q: %w", filePath, err)
		}
		keyList = append(keyList, filename)
	}
	sort.Strings(keyList)
	metadata = append(metadata, MetadataEntry{Name: "keys", Value: strings.Join(keyList, ", ")})

	currentHash := computeHash(data)
	resp := InResponse{
		Version:  Version{Hash: currentHash},
		Metadata: metadata,
	}

	return json.NewEncoder(stdout).Encode(resp)
}

func handleOut(stdin io.Reader, stdout io.Writer) error {
	var req struct {
		Source Source `json:"source"`
	}
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		return fmt.Errorf("decode out request: %w", err)
	}

	resp := OutResponse{
		Version:  Version{Hash: "latest"},
		Metadata: []MetadataEntry{},
	}
	return json.NewEncoder(stdout).Encode(resp)
}
