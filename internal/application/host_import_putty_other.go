//go:build !windows

package application

import "errors"

// parsePuTTY is unavailable off-Windows: PuTTY keeps its sessions in the
// Windows registry, there is no file to parse.
func parsePuTTY() ([]ImportedHost, error) {
	return nil, errors.New("PuTTY sessions live in the Windows registry — import them on Windows")
}
