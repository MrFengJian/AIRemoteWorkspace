package ssh

import "testing"

func TestParseOSRelease(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		{`NAME="Ubuntu"
VERSION="24.04.2 LTS (Noble Numbat)"
ID=ubuntu
ID_LIKE=debian`, "ubuntu"},
		{`NAME="Rocky Linux"
VERSION="9.5 (Blue Onyx)"
ID="rocky"
ID_LIKE="rhel centos fedora"`, "rocky"},
		{`NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.20.3`, "alpine"},
		{`NAME="Debian GNU/Linux"
ID=debian`, "debian"},
		{`NAME="Fedora Linux"
VERSION="41 (Workstation Edition)"
ID=fedora`, "fedora"},
		{`NAME="Unknown Distro"`, "unknown-distro"},
		{``, ""},
	}
	for _, c := range cases {
		got := parseOSRelease(c.content)
		if got != c.want {
			t.Errorf("parseOSRelease(%q) = %q, want %q", c.content[:min(len(c.content), 20)], got, c.want)
		}
	}
}

func TestNormalizeOSID(t *testing.T) {
	if got := normalizeOSID("Red Hat Enterprise Linux"); got != "redhat" {
		t.Errorf("RHEL normalize = %q, want redhat", got)
	}
	if got := normalizeOSID("UBUNTU"); got != "ubuntu" {
		t.Errorf("UBUNTU normalize = %q, want ubuntu", got)
	}
	if got := normalizeOSID("Rocky"); got != "rocky" {
		t.Errorf("Rocky normalize = %q, want rocky", got)
	}
}
