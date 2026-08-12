//go:build !windows && !darwin && !linux

package secret

// defaultStore is a no-op on unsupported platforms (e.g. BSD, mobile). Callers
// should detect ErrUnsupported and fall back to in-memory-only credentials.
var defaultStore Store = unsupportedStore{}

type unsupportedStore struct{}

func (unsupportedStore) Set(string, []byte) error    { return ErrUnsupported }
func (unsupportedStore) Get(string) ([]byte, error)  { return nil, ErrUnsupported }
func (unsupportedStore) Delete(string) error         { return nil }
