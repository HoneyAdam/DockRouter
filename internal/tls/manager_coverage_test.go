package tls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"testing"
	"time"
)

// TestGenerateCSRParsed tests generateCSR produces a parseable CSR.
func TestGenerateCSRParsed(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})

	csr, err := mgr.generateCSR(privKey, "test.example.com")
	if err != nil {
		t.Fatalf("generateCSR: %v", err)
	}
	if len(csr) == 0 {
		t.Error("CSR should not be empty")
	}

	parsed, err := x509.ParseCertificateRequest(csr)
	if err != nil {
		t.Fatalf("parse CSR: %v", err)
	}
	if parsed.Subject.CommonName != "test.example.com" {
		t.Errorf("CommonName = %q, want test.example.com", parsed.Subject.CommonName)
	}
	if len(parsed.DNSNames) != 1 || parsed.DNSNames[0] != "test.example.com" {
		t.Errorf("DNSNames = %v, want [test.example.com]", parsed.DNSNames)
	}
}

// TestEncodePrivateKeyRoundTrip tests encodePrivateKey produces parseable PEM.
func TestEncodePrivateKeyRoundTrip(t *testing.T) {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})

	keyPEM, err := mgr.encodePrivateKey(privKey)
	if err != nil {
		t.Fatalf("encodePrivateKey: %v", err)
	}

	block, _ := pem.Decode(keyPEM)
	if block == nil {
		t.Fatal("should decode valid PEM block")
	}
	if block.Type != "EC PRIVATE KEY" {
		t.Errorf("PEM type = %q, want EC PRIVATE KEY", block.Type)
	}

	parsedKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse EC key: %v", err)
	}
	if !parsedKey.Equal(privKey) {
		t.Error("parsed key should equal original")
	}
}

// TestNeedsRenewalWithExpiringCert tests needsRenewal with a cert expiring soon.
func TestNeedsRenewalWithExpiringCert(t *testing.T) {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().Add(10 * 24 * time.Hour),
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	leaf, _ := x509.ParseCertificate(certDER)

	cert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        leaf,
	}

	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})

	if !mgr.needsRenewal(cert) {
		t.Error("certificate expiring in 10 days should need renewal")
	}
}

// TestNeedsRenewalWithValidCert tests needsRenewal with a cert far from expiry.
func TestNeedsRenewalWithValidCert(t *testing.T) {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().Add(180 * 24 * time.Hour),
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	leaf, _ := x509.ParseCertificate(certDER)

	cert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        leaf,
	}

	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})

	if mgr.needsRenewal(cert) {
		t.Error("certificate expiring in 180 days should not need renewal")
	}
}

// TestGenerateSelfSignedWithLeaf tests GenerateSelfSigned has Leaf populated.
func TestGenerateSelfSignedWithLeaf(t *testing.T) {
	cert, err := GenerateSelfSigned("fallback.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	if cert == nil {
		t.Fatal("cert should not be nil")
	}
	if cert.Leaf == nil {
		t.Fatal("cert.Leaf should not be nil")
	}
	if cert.Leaf.Subject.CommonName != "fallback.example.com" {
		t.Errorf("CommonName = %q, want fallback.example.com", cert.Leaf.Subject.CommonName)
	}
}

// TestGenerateSelfSignedWildcard tests GenerateSelfSigned with wildcard domain.
func TestGenerateSelfSignedWildcard(t *testing.T) {
	cert, err := GenerateSelfSigned("*.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned wildcard: %v", err)
	}
	if cert.Leaf.Subject.CommonName != "*.example.com" {
		t.Errorf("CommonName = %q, want *.example.com", cert.Leaf.Subject.CommonName)
	}
}

// TestGenerateSelfSignedHasIP tests GenerateSelfSigned includes 127.0.0.1 IP.
func TestGenerateSelfSignedHasIP(t *testing.T) {
	cert, err := GenerateSelfSigned("test.local")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	if len(cert.Leaf.IPAddresses) != 1 {
		t.Fatalf("IPAddresses count = %d, want 1", len(cert.Leaf.IPAddresses))
	}
	expected := net.ParseIP("127.0.0.1")
	if !cert.Leaf.IPAddresses[0].Equal(expected) {
		t.Errorf("IP = %v, want %v", cert.Leaf.IPAddresses[0], expected)
	}
}

// TestGetAccountThumbprintNoACME tests getAccountThumbprint with nil ACME.
func TestGetAccountThumbprintNoACME(t *testing.T) {
	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})

	thumbprint := mgr.getAccountThumbprint()
	if thumbprint != "" {
		t.Errorf("thumbprint = %q, want empty when no ACME", thumbprint)
	}
}

// TestGetAccountThumbprintWithKey tests getAccountThumbprint with valid key.
func TestGetAccountThumbprintWithKey(t *testing.T) {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	store := NewStore(t.TempDir())
	acme := &ACMEClient{privateKey: privKey}
	mgr := NewManager(store, acme, nil, &mockTLSLogger{})

	thumbprint := mgr.getAccountThumbprint()
	if thumbprint == "" {
		t.Error("thumbprint should not be empty with valid key")
	}
}

// TestPadToLengthTable tests padToLength with various inputs.
func TestPadToLengthTable(t *testing.T) {
	tests := []struct {
		input  []byte
		length int
		want   int
	}{
		{[]byte{0x01}, 4, 4},
		{[]byte{0x01, 0x02, 0x03, 0x04}, 4, 4},
		{[]byte{0x01, 0x02, 0x03, 0x04, 0x05}, 4, 4},
		{[]byte{}, 4, 4},
	}
	for _, tt := range tests {
		result := padToLength(tt.input, tt.length)
		if len(result) != tt.want {
			t.Errorf("padToLength(%v, %d) length = %d, want %d", tt.input, tt.length, len(result), tt.want)
		}
	}
}

// TestComputeJWKThumbprintSameKey tests computeJWKThumbprint determinism.
func TestComputeJWKThumbprintSameKey(t *testing.T) {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	t1 := computeJWKThumbprint(privKey.PublicKey)
	t2 := computeJWKThumbprint(privKey.PublicKey)
	if t1 != t2 {
		t.Error("same key should produce same thumbprint")
	}
}

// TestComputeJWKThumbprintDifferentKeys tests different keys produce different thumbprints.
func TestComputeJWKThumbprintDifferentKeys(t *testing.T) {
	key1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	t1 := computeJWKThumbprint(key1.PublicKey)
	t2 := computeJWKThumbprint(key2.PublicKey)
	if t1 == t2 {
		t.Error("different keys should produce different thumbprints")
	}
}

// TestGetCachedCertificate tests GetCachedCertificate.
func TestGetCachedCertificate(t *testing.T) {
	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})

	cert := mgr.GetCachedCertificate("missing.example.com")
	if cert != nil {
		t.Error("should return nil for uncached domain")
	}

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	leaf, _ := x509.ParseCertificate(certDER)
	testCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        leaf,
	}
	mgr.mu.Lock()
	mgr.certs["cached.example.com"] = testCert
	mgr.mu.Unlock()

	cached := mgr.GetCachedCertificate("cached.example.com")
	if cached == nil {
		t.Error("should return cached cert")
	}
}

// TestListCertificates tests ListCertificates.
func TestListCertificates(t *testing.T) {
	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})

	domains := mgr.ListCertificates()
	if len(domains) != 0 {
		t.Errorf("domains = %d, want 0", len(domains))
	}

	mgr.mu.Lock()
	mgr.certs["a.example.com"] = &tls.Certificate{}
	mgr.certs["b.example.com"] = &tls.Certificate{}
	mgr.mu.Unlock()

	domains = mgr.ListCertificates()
	if len(domains) != 2 {
		t.Errorf("domains = %d, want 2", len(domains))
	}
}

// TestGetTLSConfig tests GetTLSConfig returns valid config.
func TestGetTLSConfig(t *testing.T) {
	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})

	config := mgr.GetTLSConfig()
	if config == nil {
		t.Fatal("config should not be nil")
	}
	if config.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %d, want TLS 1.3", config.MinVersion)
	}
	if config.GetCertificate == nil {
		t.Error("GetCertificate should not be nil")
	}
}

// TestRenewalSchedulerCheckRenewalsEmpty tests checkRenewals with no certificates.
func TestRenewalSchedulerCheckRenewalsEmpty(t *testing.T) {
	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})
	scheduler := NewRenewalScheduler(mgr, &mockTLSLogger{})
	scheduler.checkRenewals()
}

// TestRenewalSchedulerCheckRenewalsBadPEM tests checkRenewals when PEM data is corrupt.
func TestRenewalSchedulerCheckRenewalsBadPEM(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})

	// Create a domain directory but without proper cert files
	dir := tmpDir + "/certificates/broken.example.com"
	os.MkdirAll(dir, 0700)
	os.WriteFile(dir+"/cert.pem", []byte("not a real cert"), 0600)
	os.WriteFile(dir+"/key.pem", []byte("not a real key"), 0600)

	scheduler := NewRenewalScheduler(mgr, &mockTLSLogger{})
	scheduler.checkRenewals()
}

// TestRenewalSchedulerStartStopCycle tests Start and Stop lifecycle.
func TestRenewalSchedulerStartStopCycle(t *testing.T) {
	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})
	scheduler := NewRenewalScheduler(mgr, &mockTLSLogger{})
	scheduler.interval = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	scheduler.Stop()
}

// TestRenewalSchedulerStartIdempotent tests calling Start twice is safe.
func TestRenewalSchedulerStartIdempotent(t *testing.T) {
	store := NewStore(t.TempDir())
	mgr := NewManager(store, nil, nil, &mockTLSLogger{})
	scheduler := NewRenewalScheduler(mgr, &mockTLSLogger{})
	scheduler.interval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Start(ctx)
	scheduler.Start(ctx) // second call should be no-op
	time.Sleep(50 * time.Millisecond)
	scheduler.Stop()
}
