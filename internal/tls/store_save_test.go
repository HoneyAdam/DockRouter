package tls

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStoreSaveMkdirAllFail tests Save when MkdirAll fails.
func TestStoreSaveMkdirAllFail(t *testing.T) {
	// Create a file where a directory is expected so MkdirAll fails
	tmpDir := t.TempDir()
	fileAsDir := filepath.Join(tmpDir, "blocked")
	os.WriteFile(fileAsDir, []byte("blocker"), 0600)

	store := NewStore(filepath.Join(fileAsDir, "nested"))
	err := store.Save("test.example.com", []byte("cert"), []byte("key"))
	if err == nil {
		t.Error("Save should fail when MkdirAll fails")
	}
}

// TestStoreSaveCertWriteFail tests Save when writing cert.pem fails.
func TestStoreSaveCertWriteFail(t *testing.T) {
	tmpDir := t.TempDir()
	certDir := filepath.Join(tmpDir, "certificates", "test.example.com")
	os.MkdirAll(certDir, 0755)

	// Make cert.pem a directory so WriteFile fails
	os.MkdirAll(filepath.Join(certDir, "cert.pem"), 0755)

	store := NewStore(tmpDir)
	err := store.Save("test.example.com", []byte("cert"), []byte("key"))
	if err == nil {
		t.Error("Save should fail when cert.pem write fails")
	}
}

// TestStoreSaveKeyWriteFail tests Save when writing key.pem fails.
func TestStoreSaveKeyWriteFail(t *testing.T) {
	tmpDir := t.TempDir()
	certDir := filepath.Join(tmpDir, "certificates", "test.example.com")
	os.MkdirAll(certDir, 0755)

	// Write cert.pem successfully
	os.WriteFile(filepath.Join(certDir, "cert.pem"), []byte("cert"), 0600)

	// Make key.pem a directory so WriteFile fails
	os.MkdirAll(filepath.Join(certDir, "key.pem"), 0755)

	store := NewStore(tmpDir)
	err := store.Save("test.example.com", []byte("cert"), []byte("key"))
	if err == nil {
		t.Error("Save should fail when key.pem write fails")
	}
}
