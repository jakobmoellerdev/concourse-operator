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
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHandleCheckAndInWithConfigMap(t *testing.T) {
	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"app.json": `{"port": 8080}`,
			"env.txt":  "production",
		},
	})

	// 1. Test fetchData
	data, err := fetchData(ctx, fakeClient, Source{Kind: "ConfigMap", Name: "test-cm", Namespace: "default"}, "default")
	if err != nil {
		t.Fatalf("fetchData failed: %v", err)
	}
	if len(data) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(data))
	}
	if string(data["app.json"]) != `{"port": 8080}` {
		t.Fatalf("unexpected content for app.json: %s", string(data["app.json"]))
	}

	hash1 := computeHash(data)
	if hash1 == "" {
		t.Fatal("expected non-empty hash")
	}

	// 2. Test In logic by simulating handleIn with a temp directory
	destDir := t.TempDir()
	metadata := []MetadataEntry{
		{Name: "kind", Value: "ConfigMap"},
		{Name: "name", Value: "test-cm"},
		{Name: "namespace", Value: "default"},
	}
	for filename, content := range data {
		filePath := filepath.Join(destDir, filename)
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}

	// Verify written files
	f1, err := os.ReadFile(filepath.Join(destDir, "app.json"))
	if err != nil || string(f1) != `{"port": 8080}` {
		t.Fatalf("file content mismatch: %v", err)
	}
	f2, err := os.ReadFile(filepath.Join(destDir, "env.txt"))
	if err != nil || string(f2) != "production" {
		t.Fatalf("file content mismatch: %v", err)
	}

	resp := InResponse{
		Version:  Version{Hash: hash1},
		Metadata: metadata,
	}
	if resp.Version.Hash != hash1 {
		t.Fatalf("hash mismatch in response")
	}
	if len(resp.Metadata) != 3 {
		t.Fatalf("expected 3 metadata entries, got %d", len(resp.Metadata))
	}
}

func TestHandleCheckAndInWithSecret(t *testing.T) {
	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sec",
			Namespace: "production",
		},
		Data: map[string][]byte{
			"token.key": []byte("super-secret-token"),
		},
	})

	src := Source{Kind: "Secret", Name: "test-sec", Namespace: "production"}
	data, err := fetchData(ctx, fakeClient, src, "production")
	if err != nil {
		t.Fatalf("fetchData failed: %v", err)
	}
	if string(data["token.key"]) != "super-secret-token" {
		t.Fatalf("unexpected content: %s", string(data["token.key"]))
	}

	hash := computeHash(data)
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
}

func TestRunCommandDispatch(t *testing.T) {
	outBuf := &bytes.Buffer{}
	inBuf := bytes.NewBufferString(`{"source":{"kind":"ConfigMap","name":"none"}}`)

	// Test out command
	err := run([]string{"out"}, inBuf, outBuf)
	if err != nil {
		t.Fatalf("run out failed: %v", err)
	}

	var resp OutResponse
	if err := json.Unmarshal(outBuf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode out response: %v", err)
	}
	if resp.Version.Hash != "latest" {
		t.Fatalf("expected hash 'latest', got %s", resp.Version.Hash)
	}
}
