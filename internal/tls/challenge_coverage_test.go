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
	"time"
)

// TestChallengeHandlerEmptyToken tests Handler with valid path but empty token.
func TestChallengeHandlerEmptyToken(t *testing.T) {
	solver := NewChallengeSolver()
	handler := solver.Handler()

	req := httptest.NewRequest("GET", "/.well-known/acme-challenge/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for empty token", rec.Code)
	}
}

// TestChallengeHandlerInvalidPath tests Handler with non-challenge path.
func TestChallengeHandlerInvalidPath(t *testing.T) {
	solver := NewChallengeSolver()
	handler := solver.Handler()

	req := httptest.NewRequest("GET", "/other-path", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid path", rec.Code)
	}
}

// TestChallengeHandlerExactPath tests Handler with exact path (no token after prefix).
func TestChallengeHandlerExactPath(t *testing.T) {
	solver := NewChallengeSolver()
	handler := solver.Handler()

	// Path exactly equals prefix — Matches returns false because len(path) not > len(prefix)
	req := httptest.NewRequest("GET", "/.well-known/acme-challenge/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	// Either 400 (Matches false) or 400 (empty token) — both are fine
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestChallengeMatchesExactPrefix tests Matches with path exactly equal to prefix.
func TestChallengeMatchesExactPrefix(t *testing.T) {
	solver := NewChallengeSolver()
	if solver.Matches("/.well-known/acme-challenge/") {
		t.Error("Matches should return false when path == prefix (not longer)")
	}
}

// TestChallengeMatchesShortPath tests Matches with shorter path.
func TestChallengeMatchesShortPath(t *testing.T) {
	solver := NewChallengeSolver()
	if solver.Matches("/.well-known/") {
		t.Error("Matches should return false for shorter path")
	}
}

// TestChallengePathPrefix returns the correct prefix.
func TestChallengePathPrefix(t *testing.T) {
	solver := NewChallengeSolver()
	if solver.PathPrefix() != "/.well-known/acme-challenge/" {
		t.Errorf("PathPrefix = %q, want /.well-known/acme-challenge/", solver.PathPrefix())
	}
}

// TestRequestOrderBadStatus tests RequestOrder with non-2xx response.
func TestRequestOrderBadStatus(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "new-nonce")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"urn:ietf:params:acme:error:badNonce","detail":"bad nonce","status":400}`))
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"
	client.newOrderURL = server.URL + "/new-order"

	_, err := client.RequestOrder([]string{"example.com"})
	if err == nil {
		t.Fatal("RequestOrder should fail with 400")
	}
}

// TestRequestOrderBadJSON tests RequestOrder with invalid JSON response.
func TestRequestOrderBadJSON(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "new-nonce")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"
	client.newOrderURL = server.URL + "/new-order"

	_, err := client.RequestOrder([]string{"example.com"})
	if err == nil {
		t.Fatal("RequestOrder should fail with invalid JSON")
	}
}

// TestRequestOrderSuccess tests RequestOrder with valid response.
func TestRequestOrderSuccess(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "new-nonce")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(ACMEOrder{
			Status:         "pending",
			Authorizations: []string{server.URL + "/authz/1"},
			FinalizeURL:    server.URL + "/finalize/1",
		})
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"
	client.newOrderURL = server.URL + "/new-order"

	order, err := client.RequestOrder([]string{"example.com", "www.example.com"})
	if err != nil {
		t.Fatalf("RequestOrder: %v", err)
	}
	if order.Status != "pending" {
		t.Errorf("Status = %q, want pending", order.Status)
	}
	if len(order.Authorizations) != 1 {
		t.Errorf("Authorizations count = %d, want 1", len(order.Authorizations))
	}
}

// TestSignPayloadWithNil tests signPayload uses jwk when accountURL is empty.
func TestSignPayloadWithNil(t *testing.T) {
	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"
	// accountURL is empty — should use jwk

	jws, err := client.signPayload(map[string]string{"test": "data"}, "https://example.com/resource")
	if err != nil {
		t.Fatalf("signPayload: %v", err)
	}

	// Verify the protected header contains jwk (not kid)
	protectedStr, ok := jws["protected"].(string)
	if !ok {
		t.Fatal("protected should be a string")
	}
	if strings.Contains(protectedStr, "kid") {
		t.Error("protected should not contain kid when accountURL is empty")
	}
}

// TestSignedGetNonceUpdate tests signedGet updates nonce from response.
func TestSignedGetNonceUpdate(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "updated-nonce-xyz")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "initial-nonce"

	resp, err := client.signedGet(server.URL)
	if err != nil {
		t.Fatalf("signedGet: %v", err)
	}
	defer resp.Body.Close()

	if client.nonce != "updated-nonce-xyz" {
		t.Errorf("nonce = %q, want updated-nonce-xyz", client.nonce)
	}
}

// TestDownloadCertificateSuccess tests DownloadCertificate with valid response.
func TestDownloadCertificateSuccess(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "new-nonce")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("-----BEGIN CERTIFICATE-----\nMIIBtest\n-----END CERTIFICATE-----\n"))
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"

	data, err := client.DownloadCertificate(server.URL + "/cert/1")
	if err != nil {
		t.Fatalf("DownloadCertificate: %v", err)
	}
	if !strings.Contains(string(data), "BEGIN CERTIFICATE") {
		t.Errorf("data = %q, should contain certificate", string(data))
	}
}

// TestFinalizeOrderSuccess tests FinalizeOrder with valid response.
func TestFinalizeOrderSuccess(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "new-nonce")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ACMEOrder{
			Status:         "valid",
			CertificateURL: server.URL + "/cert/1",
		})
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"

	order := &ACMEOrder{FinalizeURL: server.URL + "/finalize/1"}
	err := client.FinalizeOrder(order, []byte("dummy-csr"))
	if err != nil {
		t.Fatalf("FinalizeOrder: %v", err)
	}
	if order.Status != "valid" {
		t.Errorf("Status = %q, want valid", order.Status)
	}
	if order.CertificateURL != server.URL+"/cert/1" {
		t.Errorf("CertificateURL = %q, want correct URL", order.CertificateURL)
	}
}

// TestPollOrderSuccessImmediate tests PollOrder when order is already in desired state.
func TestPollOrderSuccessImmediate(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "new-nonce")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ACMEOrder{
			Status:         "valid",
			CertificateURL: server.URL + "/cert/1",
		})
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"

	order, err := client.PollOrder(server.URL, "valid", 5*time.Second)
	if err != nil {
		t.Fatalf("PollOrder: %v", err)
	}
	if order.Status != "valid" {
		t.Errorf("Status = %q, want valid", order.Status)
	}
}
