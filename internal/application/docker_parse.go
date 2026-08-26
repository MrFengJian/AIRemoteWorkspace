package application

import (
	"encoding/json"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// docker_parse.go turns the CLI's `--format '{{json .}}'` output (one JSON
// object per line, occasionally interleaved with stderr noise since exec
// channels are combined) into domain models. Non-JSON lines are skipped.

// jsonLines yields the '{'-prefixed lines of out (the actual JSON records).
func jsonLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "{") {
			lines = append(lines, l)
		}
	}
	return lines
}

// --- raw CLI JSON shapes (docker uses exported Go field names as keys) ---

type dockerPsRow struct {
	ID        string `json:"ID"`
	Names     string `json:"Names"`
	Image     string `json:"Image"`
	State     string `json:"State"`
	Status    string `json:"Status"`
	Ports     string `json:"Ports"`
	CreatedAt string `json:"CreatedAt"`
}

type dockerStatsRow struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemPerc  string `json:"MemPerc"`
	MemUsage string `json:"MemUsage"`
	NetIO    string `json:"NetIO"`
	BlockIO  string `json:"BlockIO"`
	PIDs     string `json:"PIDs"`
}

type dockerImagesRow struct {
	Repository   string `json:"Repository"`
	Tag          string `json:"Tag"`
	ID           string `json:"ID"`
	CreatedSince string `json:"CreatedSince"`
	Size         string `json:"Size"`
}

type dockerVersionDoc struct {
	Server struct {
		Version       string `json:"Version"`
		APIVersion    string `json:"ApiVersion"`
		Os            string `json:"Os"`
		Arch          string `json:"Arch"`
		KernelVersion string `json:"KernelVersion"`
	} `json:"Server"`
}

type dockerNetworksRow struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	Driver   string `json:"Driver"`
	Scope    string `json:"Scope"`
	Internal bool   `json:"Internal"`
	IPv6     bool   `json:"IPv6"`
}

type dockerNetworkInspectDoc struct {
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Scope      string `json:"Scope"`
	Internal   bool   `json:"Internal"`
	EnableIPv6 bool   `json:"EnableIPv6"`
	IPAM       struct {
		Config []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		} `json:"Config"`
	} `json:"IPAM"`
	Containers map[string]struct {
		Name        string `json:"Name"`
		IPv4Address string `json:"IPv4Address"`
		IPv6Address string `json:"IPv6Address"`
	} `json:"Containers"`
}

type dockerInfoDoc struct {
	ContainersRunning int    `json:"ContainersRunning"`
	ContainersPaused  int    `json:"ContainersPaused"`
	ContainersStopped int    `json:"ContainersStopped"`
	Images            int    `json:"Images"`
	DockerRootDir     string `json:"DockerRootDir"`
}

// parseDockerContainers decodes `docker ps --format json` rows.
func parseDockerContainers(out string) []domain.DockerContainer {
	rows := jsonLines(out)
	containers := make([]domain.DockerContainer, 0, len(rows))
	for _, l := range rows {
		var r dockerPsRow
		if json.Unmarshal([]byte(l), &r) != nil || r.ID == "" {
			continue
		}
		containers = append(containers, domain.DockerContainer{
			ID:        r.ID,
			Names:     r.Names,
			Image:     r.Image,
			State:     r.State,
			Status:    r.Status,
			Ports:     r.Ports,
			CreatedAt: r.CreatedAt,
		})
	}
	return containers
}

// parseDockerStats decodes `docker stats --no-stream --format json` rows.
func parseDockerStats(out string) []domain.DockerContainerStats {
	rows := jsonLines(out)
	stats := make([]domain.DockerContainerStats, 0, len(rows))
	for _, l := range rows {
		var r dockerStatsRow
		if json.Unmarshal([]byte(l), &r) != nil || (r.ID == "" && r.Name == "") {
			continue
		}
		stats = append(stats, domain.DockerContainerStats{
			ContainerID: r.ID,
			Name:        r.Name,
			CPUPercent:  r.CPUPerc,
			MemPercent:  r.MemPerc,
			MemUsage:    r.MemUsage,
			NetIO:       r.NetIO,
			BlockIO:     r.BlockIO,
			PIDs:        r.PIDs,
		})
	}
	return stats
}

// parseDockerImages decodes `docker images --format json` rows.
func parseDockerImages(out string) []domain.DockerImage {
	rows := jsonLines(out)
	images := make([]domain.DockerImage, 0, len(rows))
	for _, l := range rows {
		var r dockerImagesRow
		if json.Unmarshal([]byte(l), &r) != nil || (r.Repository == "" && r.ID == "") {
			continue
		}
		images = append(images, domain.DockerImage{
			Repository:   r.Repository,
			Tag:          r.Tag,
			ID:           r.ID,
			CreatedSince: r.CreatedSince,
			Size:         r.Size,
		})
	}
	return images
}

// parseDockerNetworks decodes `docker network ls --format json` rows.
func parseDockerNetworks(out string) []domain.DockerNetwork {
	rows := jsonLines(out)
	networks := make([]domain.DockerNetwork, 0, len(rows))
	for _, l := range rows {
		var r dockerNetworksRow
		if json.Unmarshal([]byte(l), &r) != nil || r.Name == "" {
			continue
		}
		networks = append(networks, domain.DockerNetwork{
			ID:       r.ID,
			Name:     r.Name,
			Driver:   r.Driver,
			Scope:    r.Scope,
			Internal: r.Internal,
			IPv6:     r.IPv6,
		})
	}
	return networks
}

// parseDockerNetworkInspect decodes one `docker network inspect --format json`
// document into the flattened detail model.
func parseDockerNetworkInspect(out string) (domain.DockerNetworkDetail, bool) {
	for _, l := range jsonLines(out) {
		var d dockerNetworkInspectDoc
		if json.Unmarshal([]byte(l), &d) != nil || d.Name == "" {
			continue
		}
		detail := domain.DockerNetworkDetail{
			ID:         d.ID,
			Name:       d.Name,
			Driver:     d.Driver,
			Scope:      d.Scope,
			Internal:   d.Internal,
			EnableIPv6: d.EnableIPv6,
		}
		for _, c := range d.IPAM.Config {
			detail.Subnets = append(detail.Subnets, domain.DockerNetworkSubnet{
				Subnet:  c.Subnet,
				Gateway: c.Gateway,
			})
		}
		for _, c := range d.Containers {
			detail.Containers = append(detail.Containers, domain.DockerNetworkContainer{
				Name:        c.Name,
				IPv4Address: c.IPv4Address,
				IPv6Address: c.IPv6Address,
			})
		}
		return detail, true
	}
	return domain.DockerNetworkDetail{}, false
}

// parseDockerInfo merges `docker version` (server section) with the counters
// of `docker info`. An empty infoOut (info failed) still yields version data.
func parseDockerInfo(verOut, infoOut string) domain.DockerInfo {
	info := domain.DockerInfo{}
	for _, l := range jsonLines(verOut) {
		var v dockerVersionDoc
		if json.Unmarshal([]byte(l), &v) == nil && v.Server.Version != "" {
			info.Version = v.Server.Version
			info.APIVersion = v.Server.APIVersion
			info.OSType = v.Server.Os
			info.Arch = v.Server.Arch
			info.KernelVersion = v.Server.KernelVersion
			break
		}
	}
	for _, l := range jsonLines(infoOut) {
		var d dockerInfoDoc
		if json.Unmarshal([]byte(l), &d) == nil {
			info.ContainersRunning = d.ContainersRunning
			info.ContainersPaused = d.ContainersPaused
			info.ContainersStopped = d.ContainersStopped
			info.Images = d.Images
			info.DockerRootDir = d.DockerRootDir
			break
		}
	}
	return info
}
