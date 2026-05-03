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

// TestParseECDSASignatureTooShort tests DER input shorter than 8 bytes.
func TestParseECDSASignatureTooShort(t *testing.T) {
	_, err := parseECDSASignature([]byte{0x30, 0x02, 0x01})
	if err == nil {
		t.Error("should reject DER shorter than 8 bytes")
	}
}

// TestParseECDSASignatureWrongTag tests DER with wrong SEQUENCE tag.
func TestParseECDSASignatureWrongTag(t *testing.T) {
	der := []byte{0x31, 0x06, 0x02, 0x01, 0x01, 0x02, 0x01, 0x01}
	_, err := parseECDSASignature(der)
	if err == nil {
		t.Error("should reject DER with wrong SEQUENCE tag (0x31)")
	}
}

// TestParseECDSASignatureWrongRTag tests DER with wrong R INTEGER tag.
func TestParseECDSASignatureWrongRTag(t *testing.T) {
	der := []byte{0x30, 0x06, 0x03, 0x01, 0x01, 0x02, 0x01, 0x01}
	_, err := parseECDSASignature(der)
	if err == nil {
		t.Error("should reject DER with wrong R INTEGER tag")
	}
}

// TestParseECDSASignatureROverflow tests DER where R length exceeds data.
func TestParseECDSASignatureROverflow(t *testing.T) {
	der := []byte{0x30, 0x06, 0x02, 0x10, 0x01, 0x02, 0x01, 0x01}
	_, err := parseECDSASignature(der)
	if err == nil {
		t.Error("should reject DER where R length exceeds data")
	}
}

// TestParseECDSASignatureWrongSTag tests DER with wrong S INTEGER tag.
func TestParseECDSASignatureWrongSTag(t *testing.T) {
	der := []byte{0x30, 0x06, 0x02, 0x01, 0x01, 0x03, 0x01, 0x01}
	_, err := parseECDSASignature(der)
	if err == nil {
		t.Error("should reject DER with wrong S INTEGER tag")
	}
}

// TestParseECDSASignatureSOverflow tests DER where S length exceeds data.
func TestParseECDSASignatureSOverflow(t *testing.T) {
	der := []byte{0x30, 0x06, 0x02, 0x01, 0x01, 0x02, 0x10, 0x01}
	_, err := parseECDSASignature(der)
	if err == nil {
		t.Error("should reject DER where S length exceeds data")
	}
}

// TestParseECDSASignatureValidShort tests valid DER with short R and S values.
func TestParseECDSASignatureValidShort(t *testing.T) {
	// 0x30 <len> 0x02 <r_len> <r=0x01> 0x02 <s_len> <s=0x02>
	der := []byte{0x30, 0x06, 0x02, 0x01, 0x01, 0x02, 0x01, 0x02}
	rs, err := parseECDSASignature(der)
	if err != nil {
		t.Fatalf("parseECDSASignature valid short: %v", err)
	}
	if len(rs) != 64 {
		t.Errorf("result length = %d, want 64", len(rs))
	}
}

// TestPadBytesShort tests padBytes with input shorter than target.
func TestPadBytesShort(t *testing.T) {
	result := padBytes([]byte{0x01, 0x02}, 4)
	if len(result) != 4 {
		t.Errorf("padBytes length = %d, want 4", len(result))
	}
	expected := []byte{0x00, 0x00, 0x01, 0x02}
	for i, b := range result {
		if b != expected[i] {
			t.Errorf("padBytes[%d] = %d, want %d", i, b, expected[i])
		}
	}
}

// TestPadBytesExact tests padBytes with input equal to target.
func TestPadBytesExact(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03, 0x04}
	result := padBytes(input, 4)
	if len(result) != 4 {
		t.Errorf("padBytes length = %d, want 4", len(result))
	}
	for i, b := range result {
		if b != input[i] {
			t.Errorf("padBytes[%d] = %d, want %d", i, b, input[i])
		}
	}
}

// TestPadBytesLonger tests padBytes with input longer than target.
func TestPadBytesLonger(t *testing.T) {
	input := []byte{0xFF, 0x01, 0x02, 0x03, 0x04, 0x05}
	result := padBytes(input, 4)
	if len(result) != 4 {
		t.Errorf("padBytes length = %d, want 4", len(result))
	}
	// Should take the last 4 bytes
	expected := []byte{0x02, 0x03, 0x04, 0x05}
	for i, b := range result {
		if b != expected[i] {
			t.Errorf("padBytes[%d] = %d, want %d", i, b, expected[i])
		}
	}
}

// TestACMEClientInitializeFetchDirectoryFail tests Initialize when fetchDirectory returns error.
func TestACMEClientInitializeFetchDirectoryFail(t *testing.T) {
	client := NewACMEClient("://bad-url", "test@example.com")
	err := client.Initialize()
	if err == nil {
		t.Fatal("Initialize should fail with bad directory URL")
	}
	if !strings.Contains(err.Error(), "failed to fetch directory") {
		t.Errorf("error = %v, want fetch directory failure", err)
	}
}

// TestACMEClientInitializeFetchNonceFail tests Initialize when fetchNonce returns error.
func TestACMEClientInitializeFetchNonceFail(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ACMEDirectory{
			NewNonce:   "://bad-nonce-url",
			NewAccount: server.URL + "/account",
			NewOrder:   server.URL + "/order",
		})
	}))
	defer server.Close()

	client := NewACMEClient(server.URL, "test@example.com")
	err := client.Initialize()
	if err == nil {
		t.Fatal("Initialize should fail when fetchNonce fails")
	}
	if !strings.Contains(err.Error(), "failed to get nonce") {
		t.Errorf("error = %v, want nonce failure", err)
	}
}

// TestACMEClientInitializeCreateAccountFail tests Initialize when createOrGetAccount fails.
func TestACMEClientInitializeCreateAccountFail(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/":
			json.NewEncoder(w).Encode(ACMEDirectory{
				NewNonce:   server.URL + "/nonce",
				NewAccount: server.URL + "/account",
				NewOrder:   server.URL + "/order",
			})
		case "/nonce":
			w.Header().Set("Replay-Nonce", "test-nonce")
			w.WriteHeader(http.StatusOK)
		case "/account":
			// Return 500 to cause failure
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"type":"urn:ietf:params:acme:error:serverInternal","detail":"internal error","status":500}`))
		}
	}))
	defer server.Close()

	client := NewACMEClient(server.URL, "test@example.com")
	err := client.Initialize()
	if err == nil {
		t.Fatal("Initialize should fail when createOrGetAccount fails")
	}
	if !strings.Contains(err.Error(), "failed to create account") {
		t.Errorf("error = %v, want account creation failure", err)
	}
}

// TestACMEClientGetChallengeBadStatus tests GetChallenge with non-2xx response.
func TestACMEClientGetChallengeBadStatus(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "new-nonce")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"type":"urn:ietf:params:acme:error:unauthorized","detail":"invalid signature","status":403}`))
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"

	_, err := client.GetChallenge(server.URL + "/challenge/1")
	if err == nil {
		t.Fatal("GetChallenge should fail with 403 response")
	}
}

// TestACMEClientGetChallengeBadJSON tests GetChallenge with invalid JSON response.
func TestACMEClientGetChallengeBadJSON(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "new-nonce")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"

	_, err := client.GetChallenge(server.URL + "/challenge/1")
	if err == nil {
		t.Fatal("GetChallenge should fail with invalid JSON")
	}
}

// TestACMEClientPollOrderHTTPError tests PollOrder with HTTP error response.
func TestACMEClientPollOrderHTTPError(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"

	_, err := client.PollOrder(server.URL, "valid", 5*time.Second)
	if err == nil {
		t.Fatal("PollOrder should fail with 500 response")
	}
}

// TestACMEClientPollOrderBadJSON tests PollOrder with invalid JSON response.
func TestACMEClientPollOrderBadJSON(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"

	_, err := client.PollOrder(server.URL, "valid", 5*time.Second)
	if err == nil {
		t.Fatal("PollOrder should fail with invalid JSON")
	}
}

// TestSignPayloadNilPayload tests signPayload with nil payload (POST-as-GET).
func TestSignPayloadNilPayload(t *testing.T) {
	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"

	jws, err := client.signPayload(nil, "https://example.com/test")
	if err != nil {
		t.Fatalf("signPayload with nil payload: %v", err)
	}

	// Verify payload is empty string (base64 of empty bytes)
	if jws["payload"] != "" {
		t.Error("payload should be empty string for nil payload")
	}
}

// TestSignPayloadWithAccountURL tests that kid is used when accountURL is set.
func TestSignPayloadWithAccountURL(t *testing.T) {
	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"
	client.accountURL = "https://acme.example.com/account/123"

	jws, err := client.signPayload(map[string]string{"test": "data"}, "https://example.com/test")
	if err != nil {
		t.Fatalf("signPayload with account URL: %v", err)
	}
	if jws["protected"] == nil {
		t.Error("protected header should not be nil")
	}
}

// TestBase64URLEncodeEmpty tests base64URLEncode with empty input.
func TestBase64URLEncodeEmpty(t *testing.T) {
	result := base64URLEncode([]byte{})
	if result != "" {
		t.Errorf("base64URLEncode([]) = %q, want empty string", result)
	}
}

// TestCheckResponseStatusGenericErrBody tests checkResponseStatus with non-ACME error body.
func TestCheckResponseStatusGenericErrBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`not acme error format`))
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	err = checkResponseStatus(resp)
	if err == nil {
		t.Fatal("checkResponseStatus should return error for 400")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("error = %v, want generic HTTP error", err)
	}
}

// TestCheckResponseStatusACMEErrBody tests checkResponseStatus with ACME error body.
func TestCheckResponseStatusACMEErrBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"urn:ietf:params:acme:error:badNonce","detail":"bad nonce","status":400}`))
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	err = checkResponseStatus(resp)
	if err == nil {
		t.Fatal("checkResponseStatus should return error for 400")
	}
	acmeErr, ok := err.(*ACMEError)
	if !ok {
		t.Errorf("error type = %T, want *ACMEError", err)
	}
	if acmeErr.Type != "urn:ietf:params:acme:error:badNonce" {
		t.Errorf("ACMEError.Type = %s, want badNonce", acmeErr.Type)
	}
}

// TestSignedPostNoNonceUpdate tests signedPost when response has no Replay-Nonce.
func TestSignedPostNoNonceUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No Replay-Nonce header
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "initial-nonce"

	resp, err := client.signedPost(server.URL, map[string]string{"test": "data"})
	if err != nil {
		t.Fatalf("signedPost: %v", err)
	}
	defer resp.Body.Close()

	if client.nonce != "initial-nonce" {
		t.Errorf("nonce = %s, want initial-nonce (unchanged)", client.nonce)
	}
}

// TestACMEErrorMessage tests the ACMEError.Error() method.
func TestACMEErrorMessage(t *testing.T) {
	err := &ACMEError{
		Type:   "urn:ietf:params:acme:error:unauthorized",
		Detail: "account deactivated",
		Status: 403,
	}
	msg := err.Error()
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("Error() = %q, should contain type", msg)
	}
	if !strings.Contains(msg, "account deactivated") {
		t.Errorf("Error() = %q, should contain detail", msg)
	}
}

// TestCreateOrGetAccountNoLocation tests createOrGetAccount when Location header is empty.
func TestCreateOrGetAccountNoLocation(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No Location header
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status": "valid"}`))
	}))
	defer server.Close()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(server.URL, "test@example.com")
	client.privateKey = acmeKey
	client.nonce = "test-nonce"
	client.newAccountURL = server.URL + "/account"

	err := client.createOrGetAccount()
	if err != nil {
		t.Fatalf("createOrGetAccount: %v", err)
	}
	if client.accountURL != "" {
		t.Errorf("accountURL = %q, want empty when no Location header", client.accountURL)
	}
}
