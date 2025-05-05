// Package enterprisereports provides functionality for generating reports about GitHub Enterprise resources.
package enterprisereports

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func generateTestRSAKey(t *testing.T) (*rsa.PrivateKey, string) {
	// Generate a new private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Encode to PEM format
	privKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privKeyBytes,
	})

	// Write to temp file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_private_key.pem")

	err = os.WriteFile(keyPath, privKeyPEM, 0600)
	if err != nil {
		t.Fatalf("Failed to write test private key file: %v", err)
	}

	return privateKey, keyPath
}

func TestNewRESTClient_Token(t *testing.T) {
	ctx := context.Background()
	conf := &Config{
		AuthMethod: "token",
		Token:      "test-token",
	}

	client, err := NewRESTClient(ctx, conf)
	if err != nil {
		t.Fatalf("NewRESTClient() error = %v", err)
	}

	if client == nil {
		t.Fatal("Expected client to be non-nil")
	}

	// Test that the token is used in requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token != "Bearer test-token" {
			t.Errorf("Expected Authorization header 'Bearer test-token', got %q", token)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	// Update the client's base URL to point to our test server
	client.BaseURL, _ = url.Parse(server.URL + "/")

	// Make a test request to rate_limit endpoint which is unlikely to change
	_, _, err = client.RateLimit.Get(ctx)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
}

func TestNewRESTClient_App(t *testing.T) {
	ctx := context.Background()
	_, keyPath := generateTestRSAKey(t)

	conf := &Config{
		AuthMethod:              "app",
		GithubAppID:             123,
		GithubAppInstallationID: 456,
		GithubAppPrivateKey:     keyPath,
	}

	client, err := NewRESTClient(ctx, conf)
	if err != nil {
		t.Fatalf("NewRESTClient() error = %v", err)
	}

	if client == nil {
		t.Fatal("Expected client to be non-nil")
	}
}

func TestNewRESTClient_InvalidAuthMethod(t *testing.T) {
	ctx := context.Background()
	conf := &Config{
		AuthMethod: "invalid",
	}

	_, err := NewRESTClient(ctx, conf)
	if err == nil {
		t.Fatal("Expected error for invalid auth method, got nil")
	}

	expectedErrPrefix := "unsupported auth-method \"invalid\""
	if err.Error()[:len(expectedErrPrefix)] != expectedErrPrefix {
		t.Errorf("Expected error message starting with %q, got %q", expectedErrPrefix, err.Error())
	}
}

func TestNewGraphQLClient_Token(t *testing.T) {
	ctx := context.Background()
	conf := &Config{
		AuthMethod: "token",
		Token:      "test-token",
	}

	client, err := NewGraphQLClient(ctx, conf)
	if err != nil {
		t.Fatalf("NewGraphQLClient() error = %v", err)
	}

	if client == nil {
		t.Fatal("Expected client to be non-nil")
	}
}

func TestNewGraphQLClient_App(t *testing.T) {
	ctx := context.Background()
	_, keyPath := generateTestRSAKey(t)

	conf := &Config{
		AuthMethod:              "app",
		GithubAppID:             123,
		GithubAppInstallationID: 456,
		GithubAppPrivateKey:     keyPath,
	}

	client, err := NewGraphQLClient(ctx, conf)
	if err != nil {
		t.Fatalf("NewGraphQLClient() error = %v", err)
	}

	if client == nil {
		t.Fatal("Expected client to be non-nil")
	}
}

func TestNewGraphQLClient_InvalidAuthMethod(t *testing.T) {
	ctx := context.Background()
	conf := &Config{
		AuthMethod: "invalid",
	}

	_, err := NewGraphQLClient(ctx, conf)
	if err == nil {
		t.Fatal("Expected error for invalid auth method, got nil")
	}

	expectedErrPrefix := "unsupported auth-method \"invalid\""
	if err.Error()[:len(expectedErrPrefix)] != expectedErrPrefix {
		t.Errorf("Expected error message starting with %q, got %q", expectedErrPrefix, err.Error())
	}
}
