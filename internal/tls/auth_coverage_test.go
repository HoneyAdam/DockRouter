package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProcessAuthorizationNoHTTP01Challenge tests the "no HTTP-01 challenge" error path.
func TestProcessAuthorizationNoHTTP01Challenge(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "test-nonce")
		switch {
		case strings.Contains(r.URL.Path, "/authz/"):
			json.NewEncoder(w).Encode(ACMEAuthorization{
				Status:     "pending",
				Identifier: Identifier{Type: "dns", Value: "test.example.com"},
				Challenges: []Challenge{
					{Type: "dns-01", URL: server.URL + "/challenge/1", Token: "tok", Status: "pending"},
				},
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, acme, NewChallengeSolver(), logger)

	err := manager.processAuthorization(server.URL + "/authz/1")
	if err == nil {
		t.Fatal("processAuthorization should fail when no HTTP-01 challenge available")
	}
	if !strings.Contains(err.Error(), "no HTTP-01 challenge") {
		t.Errorf("error = %v, want no HTTP-01 challenge", err)
	}
}

// TestProcessAuthorizationChallengeValidQuick tests the happy path where challenge
// becomes valid immediately after triggering.
func TestProcessAuthorizationChallengeValidQuick(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "test-nonce")
		switch {
		case strings.Contains(r.URL.Path, "/authz/"):
			json.NewEncoder(w).Encode(ACMEAuthorization{
				Status:     "pending",
				Identifier: Identifier{Type: "dns", Value: "test.example.com"},
				Challenges: []Challenge{
					{Type: "http-01", URL: server.URL + "/challenge/1", Token: "test-token", Status: "pending"},
				},
			})
		case strings.Contains(r.URL.Path, "/challenge/"):
			json.NewEncoder(w).Encode(Challenge{
				Type:   "http-01",
				URL:    server.URL + "/challenge/1",
				Token:  "test-token",
				Status: "valid",
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, acme, NewChallengeSolver(), logger)

	err := manager.processAuthorization(server.URL + "/authz/1")
	if err != nil {
		t.Fatalf("processAuthorization valid challenge: %v", err)
	}
}

// TestProcessAuthorizationChallengeInvalidPath tests the invalid challenge path.
func TestProcessAuthorizationChallengeInvalidPath(t *testing.T) {
	callCount := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "test-nonce")
		switch {
		case strings.Contains(r.URL.Path, "/authz/"):
			json.NewEncoder(w).Encode(ACMEAuthorization{
				Status:     "pending",
				Identifier: Identifier{Type: "dns", Value: "test.example.com"},
				Challenges: []Challenge{
					{Type: "http-01", URL: server.URL + "/challenge/1", Token: "test-token", Status: "pending"},
				},
			})
		case strings.Contains(r.URL.Path, "/challenge/"):
			callCount++
			if callCount == 1 {
				json.NewEncoder(w).Encode(Challenge{
					Type:   "http-01",
					Token:  "test-token",
					Status: "processing",
				})
			} else {
				json.NewEncoder(w).Encode(Challenge{
					Type:   "http-01",
					Token:  "test-token",
					Status: "invalid",
					Error:  &ACMEError{Type: "urn:ietf:params:acme:error:unauthorized", Detail: "unauthorized", Status: 403},
				})
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, acme, NewChallengeSolver(), logger)

	err := manager.processAuthorization(server.URL + "/authz/1")
	if err == nil {
		t.Fatal("processAuthorization should fail when challenge is invalid")
	}
	if !strings.Contains(err.Error(), "challenge failed") {
		t.Errorf("error = %v, want challenge failed", err)
	}
}

// TestProcessAuthorizationGetAuthFail tests GetAuthorization returning an error.
func TestProcessAuthorizationGetAuthFail(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "test-nonce")
		if strings.Contains(r.URL.Path, "/authz/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, acme, NewChallengeSolver(), logger)

	err := manager.processAuthorization(server.URL + "/authz/1")
	if err == nil {
		t.Fatal("processAuthorization should fail when GetAuthorization returns 500")
	}
}

// TestProcessAuthorizationTriggerFail tests TriggerChallenge failure.
func TestProcessAuthorizationTriggerFail(t *testing.T) {
	triggerCalled := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "test-nonce")
		switch {
		case strings.Contains(r.URL.Path, "/authz/"):
			json.NewEncoder(w).Encode(ACMEAuthorization{
				Status:     "pending",
				Identifier: Identifier{Type: "dns", Value: "test.example.com"},
				Challenges: []Challenge{
					{Type: "http-01", URL: server.URL + "/challenge/1", Token: "test-token", Status: "pending"},
				},
			})
		case strings.Contains(r.URL.Path, "/challenge/"):
			if !triggerCalled {
				triggerCalled = true
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(Challenge{Status: "valid"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, acme, NewChallengeSolver(), logger)

	err := manager.processAuthorization(server.URL + "/authz/1")
	if err == nil {
		t.Fatal("processAuthorization should fail when TriggerChallenge returns 500")
	}
}
