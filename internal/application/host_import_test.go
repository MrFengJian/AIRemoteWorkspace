package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/ai-remote/workspace/internal/domain"
)

// sampleXsh is a realistic Xshell 7/8 session (INI, key auth variant).
const sampleXsh = `[CONNECTION]
Host=192.168.1.10
Port=2222
Protocol=SSH

[CONNECTION:AUTHENTICATION]
UserName=deploy
Password=
UserKey=my-ed25519
Method=1

[SESSION]
Connected=1
`

// sampleXshUTF16 is the same shape as Xshell writes it by default: UTF-16 LE.
const sampleXshUTF16 = "[CONNECTION]\r\nHost=10.0.0.5\r\nPort=22\r\nProtocol=SSH\r\n\r\n[CONNECTION:AUTHENTICATION]\r\nUserName=root\r\nMethod=0\r\n"

func utf16LEWithBOM(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2, 2+len(u)*2)
	b[0], b[1] = 0xFF, 0xFE // little-endian BOM
	for _, r := range u {
		b = append(b, byte(r), byte(r>>8))
	}
	return b
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseXshellFile(t *testing.T) {
	h, ok := parseXshellFile(writeTemp(t, "web-01.xsh", sampleXsh))
	if !ok {
		t.Fatal("expected SSH session to parse")
	}
	if h.Name != "web-01" || h.Host != "192.168.1.10" || h.Port != 2222 || h.Username != "deploy" {
		t.Fatalf("unexpected fields: %+v", h)
	}
	if h.AuthType != domain.AuthKey {
		t.Fatalf("UserKey reference should map to key auth, got %q", h.AuthType)
	}
	if h.KeyPath != "" {
		t.Fatalf("xsh key names are vault-internal, want empty path, got %q", h.KeyPath)
	}
}

func TestParseXshellUTF16(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.xsh")
	if err := os.WriteFile(path, utf16LEWithBOM(sampleXshUTF16), 0o600); err != nil {
		t.Fatal(err)
	}
	h, ok := parseXshellFile(path)
	if !ok {
		t.Fatal("expected UTF-16 session to parse")
	}
	if h.Host != "10.0.0.5" || h.Username != "root" || h.Port != 22 || h.AuthType != domain.AuthPassword {
		t.Fatalf("unexpected fields: %+v", h)
	}
}

func TestParseXshellSkipsTelnet(t *testing.T) {
	telnet := strings.Replace(sampleXsh, "Protocol=SSH", "Protocol=TELNET", 1)
	if _, ok := parseXshellFile(writeTemp(t, "switch.xsh", telnet)); ok {
		t.Fatal("non-SSH sessions must be skipped")
	}
}

func TestParseXshellFolderMapsGroups(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "production")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.xsh"), []byte(sampleXsh), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.xsh"), []byte(sampleXshUTF16), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := parseXshell(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 hosts, got %d: %+v", len(got), got)
	}
	byGroup := map[string]string{}
	for _, h := range got {
		byGroup[h.Group] = h.Name
	}
	if byGroup["production"] != "a" || byGroup[""] != "b" {
		t.Fatalf("unexpected group mapping: %+v", byGroup)
	}
}

// sampleMoba mirrors a real .mxtsessions export: three SSH bookmarks in a
// subfolder, one with an identity file (MobaXterm escapes the drive as
// _CurrentDrive_), plus a VNC line to be skipped.
const sampleMoba = `[Bookmarks]
SubRep=
ImgNum=42

[Bookmarks_1]
Private=1
SubRep=Producers\Edge
ImgNum=41
edge-01=#109#0%10.20.0.1%22%ops%-1%-1%%%22%%0%0%0%%%-%0%-1%
edge-02=#109#0%10.20.0.2%2022%ops%-1%-1%%%22%%0%0%0%%%-%0%-1%
keyhost=#109#0%10.20.0.3%22%deploy%-1%-1%%%22%%0%0%0%%%-%0%_CurrentDrive_\Users\me\keys\id_rsa%-1%
vnc-box=#128#5%10.20.0.9%5900%%%
`

func TestParseMobaXterm(t *testing.T) {
	got, err := parseMobaXterm(writeTemp(t, "sessions.mxtsessions", sampleMoba))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 SSH hosts (VNC skipped), got %d: %+v", len(got), got)
	}
	byName := map[string]ImportedHost{}
	for _, h := range got {
		byName[h.Name] = h
	}
	h := byName["edge-01"]
	if h.Host != "10.20.0.1" || h.Port != 22 || h.Username != "ops" || h.Group != "Producers/Edge" {
		t.Fatalf("unexpected edge-01: %+v", h)
	}
	if h.AuthType != domain.AuthPassword {
		t.Fatalf("password session mapped to %q", h.AuthType)
	}
	if h2 := byName["edge-02"]; h2.Port != 2022 {
		t.Fatalf("unexpected port: %+v", h2)
	}
	if h3 := byName["keyhost"]; h3.AuthType != domain.AuthKey ||
		!strings.HasSuffix(h3.KeyPath, `C:\Users\me\keys\id_rsa`) {
		t.Fatalf("unexpected keyhost: %+v", h3)
	}
}

func TestParseSSHConfig(t *testing.T) {
	cfg := `# global defaults
Host *
  Compression yes
  User ops
  ServerAliveInterval 60

Host web1 web2
  HostName 10.1.0.9
  Port 2200

Host db
  HostName db.internal
  User dba
  IdentityFile ~/.ssh/id_ed25519

Host *-template !ignore
  HostName 0.0.0.0

Match host secret
  User nobody
`
	got, err := parseSSHConfigText(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want web1, web2, db (wildcard/match skipped), got %+v", got)
	}
	byName := map[string]ImportedHost{}
	for _, h := range got {
		byName[h.Name] = h
	}
	w := byName["web1"]
	if w.Host != "10.1.0.9" || w.Port != 2200 || w.Username != "ops" {
		t.Fatalf("defaults not applied: %+v", w)
	}
	d := byName["db"]
	if d.Username != "dba" || d.AuthType != domain.AuthKey ||
		!strings.HasSuffix(filepath.ToSlash(d.KeyPath), ".ssh/id_ed25519") {
		t.Fatalf("unexpected db: %+v", d)
	}
}

func TestParseSSHConfigMissing(t *testing.T) {
	if _, err := parseSSHConfig(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestDecodeSessionTextEncodings(t *testing.T) {
	if got := decodeSessionText(append([]byte{0xEF, 0xBB, 0xBF}, []byte("héllo")...)); got != "héllo" {
		t.Fatalf("utf8 bom: %q", got)
	}
	if got := decodeSessionText(utf16LEWithBOM("abc")); got != "abc" {
		t.Fatalf("utf16le: %q", got)
	}
	if got := decodeSessionText([]byte("纯文本")); got != "纯文本" {
		t.Fatalf("plain utf8: %q", got)
	}
	if got := decodeSessionText([]byte{0xD6, 0xD0}); got != "中" { // GBK 中
		t.Fatalf("gbk: %q", got)
	}
}

func TestImportedHostValid(t *testing.T) {
	ok := ImportedHost{Name: "n", Host: "h", Port: 22, Username: "u", AuthType: domain.AuthPassword}
	if !ok.valid() {
		t.Fatal("expected valid")
	}
	bad := ok
	bad.Username = ""
	if bad.valid() {
		t.Fatal("missing username must be invalid")
	}
	bad = ok
	bad.Port = 0
	if bad.valid() {
		t.Fatal("port 0 must be invalid")
	}
}

func TestParseMobaSessionUnrecognized(t *testing.T) {
	if _, ok := parseMobaSession("x", "#109#4%rdp%3389%"); ok {
		t.Fatal("RDP line must be skipped")
	}
	if _, ok := parseMobaSession("x", "#109#0%%22%"); ok {
		t.Fatal("empty host must be skipped")
	}
}

func TestPuttySessionName(t *testing.T) {
	if got := puttySessionName("My%20Server"); got != "My Server" {
		t.Fatalf("got %q", got)
	}
}
