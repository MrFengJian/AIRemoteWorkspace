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
