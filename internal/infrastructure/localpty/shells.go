package localpty

// Local shell catalogue (Phase 8 本地终端增强): detect the command lines
// available on THIS machine so the UI can offer a default (AppConfig) and a
// per-tab choice. Detection is cheap enough to call on every settings/menu
// open: LookPath + a bounded wsl probe, no process trees spawned otherwise.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/text/encoding/unicode"
)

// LocalShell is one detected command line available for local terminals.
type LocalShell struct {
	// ID is stable and used as the AppConfig.LocalShell value ("pwsh",
	// "powershell", "cmd", "wsl", "gitbash", "bash", "zsh", "fish", "sh"…).
	ID string `json:"id"`
	// Name is the display label ("PowerShell", "WSL", …).
	Name string `json:"name"`
	// Program is the resolved executable path to spawn.
	Program string `json:"program"`
	// Args are the shell's startup arguments (UTF-8 bootstrap, login flag…).
	Args []string `json:"args"`
}

// ListShells returns the shells available on this machine, in preference
// order (the first entry is the effective default when nothing is pinned).
func ListShells() []LocalShell {
	switch runtime.GOOS {
	case "windows":
		return windowsShells()
	default:
		return unixShells()
	}
}

// ShellByID resolves a detected shell by id; ok is false when the id is
// unknown (deleted shell, config from another machine) — callers fall back
// to the first detected entry.
func ShellByID(id string) (LocalShell, bool) {
	for _, s := range ListShells() {
		if s.ID == id {
			return s, true
		}
	}
	return LocalShell{}, false
}

func windowsShells() []LocalShell {
	var out []LocalShell
	if path, err := exec.LookPath("pwsh.exe"); err == nil {
		out = append(out, LocalShell{ID: "pwsh", Name: "PowerShell", Program: path,
			Args: []string{"-NoLogo", "-NoExit", "-Command", psUTF8Bootstrap}})
	}
	if path, err := exec.LookPath("powershell.exe"); err == nil {
		out = append(out, LocalShell{ID: "powershell", Name: "Windows PowerShell", Program: path,
			Args: []string{"-NoLogo", "-NoExit", "-Command", psUTF8Bootstrap}})
	}
	cmdPath := os.Getenv("COMSPEC")
	if cmdPath == "" {
		if p, err := exec.LookPath("cmd.exe"); err == nil {
			cmdPath = p
		}
	}
	if cmdPath != "" {
		out = append(out, LocalShell{ID: "cmd", Name: "CMD", Program: cmdPath,
			Args: []string{"/k", "chcp 65001>nul"}})
	}
	// WSL: one entry per installed distro (`wsl -l -v`), e.g.
	// "WSL: Ubuntu-20.04". No distros, WSL not installed, or a broken
	// service → no WSL entries at all.
	out = append(out, wslShells()...)
	for _, dir := range gitBashDirs() {
		cand := filepath.Join(dir, "bin", "bash.exe")
		if _, err := os.Stat(cand); err == nil {
			out = append(out, LocalShell{ID: "gitbash", Name: "Git Bash", Program: cand,
				Args: []string{"--login", "-i"}})
			break
		}
	}
	return out
}

// hasWSLDistro … superseded by wslShells — removed.

// wslShells enumerates the installed WSL distributions via `wsl -l -v` and
// returns one shell per distro (launching a stopped distro auto-starts it).
// Any failure — wsl.exe missing, the service broken, zero distros installed
// — yields no entries, so the UI shows no WSL options.
func wslShells() []LocalShell {
	wslPath, err := exec.LookPath("wsl.exe")
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, wslPath, "-l", "-v").Output()
	if err != nil {
		return nil
	}
	// wsl.exe emits UTF-16LE (with BOM) — distro names may be non-ASCII.
	dec := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
	text, err := dec.String(string(out))
	if err != nil {
		return nil
	}
	return parseWSLList(wslPath, text)
}

// parseWSLList decodes the textual `wsl -l -v` listing: rows of
//
//	[*) NAME STATE VERSION
//
// (the "*" default marker may be glued to the name or stand alone; the
// header row fails the numeric-version check). Pure helper for tests.
func parseWSLList(wslPath, text string) []LocalShell {
	var out []LocalShell
	for _, line := range strings.Split(text, "\r\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "*" {
			fields = fields[1:]
		}
		// NAME STATE VERSION — need at least those three.
		if len(fields) < 3 {
			continue
		}
		name := strings.TrimPrefix(fields[0], "*")
		if name == "" || strings.EqualFold(name, "NAME") {
			continue
		}
		version := fields[len(fields)-1]
		if version != "1" && version != "2" {
			continue // header or localized noise, not a distro row
		}
		out = append(out, LocalShell{
			ID:      "wsl:" + name,
			Name:    "WSL: " + name + " (v" + version + ")",
			Program: wslPath,
			Args:    []string{"-d", name},
		})
	}
	return out
}

func gitBashDirs() []string {
	dirs := make([]string, 0, 3)
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
		base := os.Getenv(env)
		if base == "" {
			continue
		}
		if env == "LocalAppData" {
			dirs = append(dirs, filepath.Join(base, "Programs", "Git"))
		} else {
			dirs = append(dirs, filepath.Join(base, "Git"))
		}
	}
	return dirs
}

func unixShells() []LocalShell {
	var out []LocalShell
	seen := make(map[string]bool)
	add := func(path string) {
		if path == "" {
			return
		}
		id := strings.TrimPrefix(filepath.Base(path), "-")
		if id == "" || seen[id] || seen[path] {
			return
		}
		seen[id] = true
		out = append(out, LocalShell{ID: id, Name: id, Program: path, Args: []string{"-l"}})
	}
	// The login shell comes first — it is the user's own default.
	add(os.Getenv("SHELL"))
	if data, err := os.ReadFile("/etc/shells"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			add(line)
		}
	}
	for _, cand := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		add(cand)
	}
	return out
}
