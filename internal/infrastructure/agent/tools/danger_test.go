package tools

import "testing"

func TestClassifyCommand(t *testing.T) {
	cases := []struct {
		cmd  string
		want string // domain.Permission string value
	}{
		// Read-only diagnostics.
		{"df -h", "read"},
		{"uptime", "read"},
		{"ps aux | grep nginx", "read"},
		{"cat /var/log/syslog", "read"},
		// Command-position matching: dangerous names as ARGUMENTS stay READ.
		{"grep rm file", "read"},
		{"man rm", "read"},
		{"echo shutdown", "read"},
		{"cat notes-about-dd.txt", "read"},
		// Safe redirects must not escalate.
		{"cat x 2> /dev/null", "read"},
		{"ls /missing 2>/dev/null", "read"},
		{"cmd 2>&1", "read"},
		// Wrappers resolve to the wrapped command.
		{"sudo rm -rf /tmp/x", "dangerous"},
		{"find . | xargs rm", "dangerous"},
		{"nohup dd if=/dev/zero of=/tmp/x bs=1 count=1", "dangerous"},
		// Dangerous commands.
		{"rm -rf /", "dangerous"},
		{"shutdown -h now", "dangerous"},
		{"reboot", "dangerous"},
		{"iptables -F", "dangerous"},
		{"mkfs.ext4 /dev/sda1", "dangerous"},
		{"docker rm web", "dangerous"},
		{"docker system prune -af", "dangerous"},
		{"kubectl delete pod api", "dangerous"},
		{"chmod 777 /etc/passwd", "dangerous"},
		{"kill -9 1234", "dangerous"},
		{"init 0", "dangerous"},
		{"echo x > /dev/sda", "dangerous"},
		// Writes (recoverable).
		{"echo hi > /tmp/f", "write"},
		{"tee /tmp/f", "write"},
		{"sed -i s/a/b/ file", "write"},
		{"systemctl restart nginx", "write"},
		{"apt install htop", "write"},
		{"docker run nginx", "write"},
		// Docker lifecycle control verbs are WRITE (runtime state changes).
		{"docker restart web", "write"},
		{"docker stop web", "write"},
		{"docker start web", "write"},
		{"docker kill web", "write"},
		{"docker pause web", "write"},
		{"docker update --memory 512m web", "write"},
		{"docker compose up -d", "write"},
		{"docker compose down", "write"},
		{"docker service scale web=3", "write"},
		// Docker reads stay READ.
		{"docker ps -a", "read"},
		{"docker logs --tail 100 web", "read"},
		{"docker stats --no-stream", "read"},
		{"docker images", "read"},
		{"docker inspect web", "read"},
		{"docker version", "read"},
		{"mkdir /tmp/d", "write"},
		{"mv a b", "write"},
		// Redirect to a real file is still a write even with 2> prefix.
		{"ls 2> /tmp/err", "write"},
	}
	for _, c := range cases {
		if got := string(classifyCommand(c.cmd)); got != c.want {
			t.Errorf("classifyCommand(%q) = %s, want %s", c.cmd, got, c.want)
		}
	}
}
