package secret

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ai-remote/workspace/internal/application"
)

// TestStore_RoundTrip exercises the platform-native Default() store end to
// end: Set → Get → Delete. This writes real entries to the OS credential
// vault (Windows Credential Manager / macOS Keychain / Linux Secret Service),
// so run it on a machine where that's available.
func TestStore_RoundTrip(t *testing.T) {
	store := Default()
	key := "airemote:test:roundtrip"
	value := []byte("phase5-secret-value")

	// Clean slate.
	_ = store.Delete(key)

	if err := store.Set(key, value); err != nil {
		t.Fatalf("Set: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(key) })

	got, err := store.Get(key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, value) {
		t.Fatalf("Get returned %q, want %q", got, value)
	}

	if err := store.Delete(key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// After delete, Get must report not found.
	_, err = store.Get(key)
	if !errors.Is(err, application.ErrSecretNotFound) {
		t.Fatalf("Get after delete: want ErrSecretNotFound, got %v", err)
	}
}

// TestStore_Overwrite verifies Set replaces an existing entry.
func TestStore_Overwrite(t *testing.T) {
	store := Default()
	key := "airemote:test:overwrite"
	t.Cleanup(func() { _ = store.Delete(key) })

	if err := store.Set(key, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(key, []byte("second")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Fatalf("overwrite: got %q, want %q", got, "second")
	}
}

// TestStore_DeleteMissing verifies Delete on an absent key is not an error.
func TestStore_DeleteMissing(t *testing.T) {
	store := Default()
	if err := store.Delete("airemote:test:nonexistent"); err != nil {
		t.Fatalf("Delete missing key should be nil, got %v", err)
	}
}
