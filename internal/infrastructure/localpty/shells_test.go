package localpty

import "testing"

const wslListSample = "\r\n" +
	"  NAME            STATE           VERSION\r\n" +
	"* Ubuntu-20.04     Running         2\r\n" +
	"  Debian           Stopped        1\r\n" +
	"\r\n"

func TestParseWSLList(t *testing.T) {
	shells := parseWSLList(`C:\Windows\System32\wsl.exe`, wslListSample)
	if len(shells) != 2 {
		t.Fatalf("want 2 distros, got %d: %+v", len(shells), shells)
	}
	first, second := shells[0], shells[1]

	// The default distro (leading "*") must not be skipped or keep the star.
	if first.ID != "wsl:Ubuntu-20.04" || first.Name != "WSL: Ubuntu-20.04 (v2)" {
		t.Fatalf("default distro mis-parsed: %+v", first)
	}
	if first.Program != `C:\Windows\System32\wsl.exe` {
		t.Fatalf("program wrong: %q", first.Program)
	}
	if len(first.Args) != 2 || first.Args[0] != "-d" || first.Args[1] != "Ubuntu-20.04" {
		t.Fatalf("launch args wrong: %v", first.Args)
	}
	if second.ID != "wsl:Debian" {
		t.Fatalf("second distro mis-parsed: %+v", second)
	}
}

func TestParseWSLListHeaderAndNoise(t *testing.T) {
	// Header row (VERSION is not numeric), the glued-star form, and a
	// localized error banner with too few fields are all skipped.
	shells := parseWSLList("wsl", "NAME STATE VERSION\r\n*Ubuntu Running 2\r\n没有已安装的分发版。\r\n")
	if len(shells) != 1 {
		t.Fatalf("want exactly 1 distro, got %d: %+v", len(shells), shells)
	}
	if shells[0].ID != "wsl:Ubuntu" {
		t.Fatalf("distro mis-parsed: %+v", shells[0])
	}
}

func TestParseWSLListEmpty(t *testing.T) {
	for _, text := range []string{"", "\r\n", "没有已安装的分发版。\r\n"} {
		if shells := parseWSLList("wsl", text); len(shells) != 0 {
			t.Fatalf("expected no distros for %q, got %+v", text, shells)
		}
	}
}
