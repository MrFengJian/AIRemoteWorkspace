package sftp

import (
	"errors"
	"fmt"
	"os"
	"testing"

	sf "github.com/pkg/sftp"

	"github.com/ai-remote/workspace/internal/application"
)

// pkg/sftp normalises Stat/Open no-such-file statuses into os.ErrNotExist —
// the raw *StatusError never escapes the client — so mapNotExist must
// recognise BOTH shapes, or every "does the target exist" check on a missing
// path turns into an error instead of a clean false (the upload regression).
func TestMapNotExist(t *testing.T) {
	// The shape Client.Stat actually returns for a missing remote path.
	statMiss := fmt.Errorf("stat %q: %w", "/a/b", os.ErrNotExist)
	if !errors.Is(mapNotExist(statMiss), application.ErrNotExist) {
		t.Fatalf("normalised miss mapped to %v, want ErrNotExist sentinel", mapNotExist(statMiss))
	}

	// A raw, unnormalised status error (defence for other call sites).
	raw := &sf.StatusError{Code: uint32(sf.ErrSSHFxNoSuchFile)}
	if !errors.Is(mapNotExist(raw), application.ErrNotExist) {
		t.Fatalf("raw status miss mapped to %v, want ErrNotExist sentinel", mapNotExist(raw))
	}

	// Other failures must pass through unmapped.
	perm := fmt.Errorf("stat %q: %w", "/a/b", os.ErrPermission)
	if errors.Is(mapNotExist(perm), application.ErrNotExist) {
		t.Fatal("permission error must not map to ErrNotExist")
	}
	if mapNotExist(nil) != nil {
		t.Fatal("nil must stay nil")
	}
}
