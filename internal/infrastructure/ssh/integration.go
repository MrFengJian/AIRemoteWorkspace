package ssh

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

// Startup-time shell integration (Tabby-style): when a session opens, we
// detect the login shell, upload a small integration script over SFTP, and
// start the shell wrapped so it reports its working directory via OSC 7 from
// the very first prompt. Because the integration loads as part of the shell
// startup command, nothing is ever echoed into the session.
//
// Fallback is sacred: ANY error in detection/upload/launch falls back to a
// plain shell request — a session without cwd reporting beats a broken one.
// Files live under /tmp (self-cleaning, no user-home litter).

// Per-family integration scripts. Each sources the user's own startup files
// first (emulating the login chain we replace), then installs the hook.
const (
	bashIntegration = `# ai-remote-workspace shell integration: report cwd via OSC 7
[ -f "$HOME/.bash_profile" ] && . "$HOME/.bash_profile"
[ -f "$HOME/.bash_login" ] && . "$HOME/.bash_login"
[ -f "$HOME/.profile" ] && . "$HOME/.profile"
[ -f "$HOME/.bashrc" ] && . "$HOME/.bashrc"
__aiws_osc7(){ printf '\033]7;file://%s%s\007' "${HOSTNAME:-localhost}" "$PWD"; }
case ";${PROMPT_COMMAND:-};" in *";__aiws_osc7;"*) ;; *) PROMPT_COMMAND="__aiws_osc7${PROMPT_COMMAND:+;$PROMPT_COMMAND}";; esac
`

	zshenvWrapper = `[ -f "$HOME/.zshenv" ] && . "$HOME/.zshenv"
[ -f "$HOME/.zprofile" ] && . "$HOME/.zprofile"
`

	zshIntegration = `# ai-remote-workspace shell integration: report cwd via OSC 7
[ -f "$HOME/.zshrc" ] && . "$HOME/.zshrc"
autoload -Uz add-zsh-hook
__aiws_osc7(){ printf '\033]7;file://%s%s\007' "${HOST:-localhost}" "$PWD" }
[[ ${precmd_functions[(I)__aiws_osc7]} -eq 0 ]] && add-zsh-hook precmd __aiws_osc7
`

	fishIntegration = `# ai-remote-workspace shell integration: report cwd via OSC 7
function __aiws_osc7 --on-event fish_prompt
    printf '\033]7;file://%s%s\007' (hostname) $PWD
end
`
)

// prepareIntegration detects the login shell, uploads the matching
// integration files, and returns the wrapped command to start instead of a
// plain shell. "" means: no integration for this shell/platform — use a
// plain shell. Never fails the session.
func (c *Client) prepareIntegration(ctx context.Context) string {
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// sshd sets SHELL (the user's login shell) on every channel.
	shell, err := c.execOutput(pctx, "echo ${SHELL:-}")
	if err != nil {
		return ""
	}
	family := shellFamily(shell)
	if family == "" {
		return ""
	}

	sft, err := sftp.NewClient(c.ssh)
	if err != nil {
		return ""
	}
	defer sft.Close()

	rnd := make([]byte, 4)
	_, _ = rand.Read(rnd)
	dir := "/tmp/.aiws-" + hex.EncodeToString(rnd)
	if err := sft.MkdirAll(dir); err != nil {
		return ""
	}

	write := func(name, content string) bool {
		f, err := sft.Create(filepath.ToSlash(filepath.Join(dir, name)))
		if err != nil {
			return false
		}
		if _, err := f.Write([]byte(content)); err != nil {
			_ = f.Close()
			return false
		}
		return f.Close() == nil
	}

	switch family {
	case "bash":
		if !write("bashrc", bashIntegration) {
			return ""
		}
		return "bash --rcfile " + dir + "/bashrc -i"
	case "zsh":
		if !write("zshenv", zshenvWrapper) || !write("zshrc", zshIntegration) {
			return ""
		}
		// Non-login interactive zsh reads $ZDOTDIR/{.zshenv,.zshrc}; the
		// wrappers chain to the user's own files first.
		return "ZDOTDIR=" + dir + " zsh -i"
	case "fish":
		if !write("aiws.fish", fishIntegration) {
			return ""
		}
		return "fish -C 'source " + dir + "/aiws.fish' -i"
	default:
		return "" // sh/csh/pwsh/unknown: plain shell, passive OSC 7 only
	}
}

// shellFamily maps a shell path to an integration family ("" = unsupported).
func shellFamily(shellPath string) string {
	base := strings.TrimPrefix(strings.TrimSpace(shellPath), "-")
	if i := strings.LastIndexAny(base, "/\\"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".exe")
	switch base {
	case "bash":
		return "bash"
	case "zsh":
		return "zsh"
	case "fish":
		return "fish"
	default:
		return ""
	}
}

// execOutput runs cmd through a throwaway exec channel and returns the
// combined output trimmed.
func (c *Client) execOutput(ctx context.Context, cmd string) (string, error) {
	sess, err := c.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()

	done := make(chan struct{})
	var out []byte
	var execErr error
	go func() {
		defer close(done)
		out, execErr = sess.CombinedOutput(cmd)
	}()
	select {
	case <-done:
		if execErr != nil {
			return "", execErr
		}
		return strings.TrimSpace(string(out)), nil
	case <-ctx.Done():
		_ = sess.Close()
		return "", ctx.Err()
	}
}
