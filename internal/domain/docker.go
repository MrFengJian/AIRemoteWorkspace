package domain

// Docker data models. Everything is collected by invoking the docker CLI on
// the host behind the session (over the SSH exec channel) or on the local
// machine (Docker Desktop / local engine), always with machine-readable
// `--format '{{json .}}'` output — no SDK, no exposed API socket. Numeric
// stats keep docker's formatted strings ("83.2MiB / 3.84GiB") because the
// CLI is the source of truth and re-parsing them adds drift for no gain.

// DockerContainer is one container row (`docker ps [-a] --format json`).
type DockerContainer struct {
	ID        string `json:"id"`
	Names     string `json:"names"`
	Image     string `json:"image"`
	State     string `json:"state"`   // "running" | "exited" | "paused" | "created" | "restarting" | "dead"
	Status    string `json:"status"`  // human status incl. health/uptime ("Up 2 hours (healthy)")
	Ports     string `json:"ports"`   // "0.0.0.0:80->80/tcp, [::]:80->80/tcp"
	CreatedAt string `json:"createdAt"`
}

// DockerContainerStats is one container's live resource usage
// (`docker stats --no-stream --format json`).
type DockerContainerStats struct {
	ContainerID string `json:"containerId"`
	Name        string `json:"name"`
	CPUPercent  string `json:"cpuPercent"` // "1.24%"
	MemPercent  string `json:"memPercent"` // "6.41%"
	MemUsage    string `json:"memUsage"`   // "123.4MiB / 3.84GiB"
	NetIO       string `json:"netIO"`      // "1.2kB / 3.4kB"
	BlockIO     string `json:"blockIO"`    // "0B / 12.6kB"
	PIDs        string `json:"pids"`
}

// DockerImage is one image row (`docker images --format json`).
type DockerImage struct {
	Repository   string `json:"repository"`
	Tag          string `json:"tag"`
	ID           string `json:"id"`
	CreatedSince string `json:"createdSince"` // "2 weeks ago"
	Size         string `json:"size"`         // "104MB"
}

// DockerInfo is the panel overview: `docker version` server section merged
// with the counters from `docker info`.
type DockerInfo struct {
	Version             string `json:"version"`
	APIVersion          string `json:"apiVersion"`
	OSType              string `json:"osType"`
	Arch                string `json:"arch"`
	KernelVersion       string `json:"kernelVersion"`
	ContainersRunning   int    `json:"containersRunning"`
	ContainersPaused    int    `json:"containersPaused"`
	ContainersStopped   int    `json:"containersStopped"`
	Images              int    `json:"images"`
	DockerRootDir       string `json:"dockerRootDir"`
}
