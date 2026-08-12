package application

import (
	"crypto/rand"
	"encoding/hex"
)

// newID returns a short, opaque, url-safe id (16 hex chars = 8 random bytes).
// Sufficient collision resistance for a single-user desktop app's host list.
func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// rand.Read virtually never fails on the supported platforms; this
		// fallback keeps us from ever returning an empty id.
		return "host00000000"
	}
	return hex.EncodeToString(b)
}
