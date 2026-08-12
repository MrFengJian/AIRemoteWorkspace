package ssh

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ErrHostKeyMismatch signals that a host's key differs from the one previously
// recorded. Callers (the UI) must decide whether to trust the new key.
// The accepted key is attached so the UI can show the new fingerprint.
var ErrHostKeyMismatch = errors.New("ssh: host key mismatch")

// HostKeyStore is the persistence port for recorded host fingerprints.
// Implemented by infrastructure/sqlite.HostKeyRepo; kept as an interface so
// this package stays free of the SQLite dependency.
type HostKeyStore interface {
	// Get returns the recorded key for hostID, or ErrNotFound if none.
	Get(hostID string) (algorithm, fingerprint string, err error)
	// Upsert records or replaces the fingerprint for hostID.
	Upsert(hostID, algorithm, fingerprint string) error
}

// ErrNotFound is returned by HostKeyStore.Get when no key is recorded yet.
var ErrNotFound = errors.New("ssh: no recorded host key")

// hostKeyCallback builds an ssh.HostKeyCallback implementing a known_hosts
// policy: first connection trusts-and-records; later connections verify and
// surface mismatches via ErrHostKeyMismatch.
//
// onNewKey is invoked exactly once when a key is first recorded, so the caller
// can surface that to the user.
func hostKeyCallback(store HostKeyStore, hostID string, onNewKey func(alg, fp string)) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		alg := key.Type()
		fp := ssh.FingerprintSHA256(key)

		storedAlg, storedFP, err := store.Get(hostID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// First contact: record and trust.
				if recordErr := store.Upsert(hostID, alg, fp); recordErr != nil {
					return fmt.Errorf("record host key: %w", recordErr)
				}
				if onNewKey != nil {
					onNewKey(alg, fp)
				}
				return nil
			}
			return fmt.Errorf("lookup host key: %w", err)
		}

		// Normalize comparison: an empty stored algorithm means "any algo,
		// match on fingerprint only" (defensive; we always store algo).
		if strings.EqualFold(storedFP, fp) && (storedAlg == "" || strings.EqualFold(storedAlg, alg)) {
			touch(store, hostID)
			return nil
		}
		return fmt.Errorf(
			"%w: recorded %s %s, got %s %s",
			ErrHostKeyMismatch, storedAlg, storedFP, alg, fp,
		)
	}
}

// touch updates last_seen without surfacing errors (best-effort).
func touch(store HostKeyStore, hostID string) {
	alg, fp, err := store.Get(hostID)
	if err != nil {
		return
	}
	_ = store.Upsert(hostID, alg, fp)
}

// formatAddr returns host:port suitable for ssh.Dial.
func formatAddr(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}

// parseRFC3339 is a tiny helper kept for symmetry with storage; unused now but
// reserved for future "first seen" display.
func parseRFC3339(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}
