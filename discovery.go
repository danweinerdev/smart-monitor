package main

import (
	"fmt"
	"path/filepath"
	"sort"
)

type DriveType string

const (
	DriveTypeSATA DriveType = "sata"
	DriveTypeNVMe DriveType = "nvme"
)

type Drive struct {
	Path string
	Name string
	Type DriveType
}

// DriveDiscoverer abstracts drive discovery.
type DriveDiscoverer interface {
	Discover() ([]Drive, error)
}

// DevFSDiscoverer discovers drives by globbing /dev.
type DevFSDiscoverer struct {
	BasePath string
}

func NewDevFSDiscoverer(basePath string) *DevFSDiscoverer {
	if basePath == "" {
		basePath = "/dev"
	}
	return &DevFSDiscoverer{BasePath: basePath}
}

func (d *DevFSDiscoverer) Discover() ([]Drive, error) {
	var drives []Drive

	sataMatches, err := filepath.Glob(filepath.Join(d.BasePath, "sata[0-9]*"))
	if err != nil {
		return nil, fmt.Errorf("globbing SATA drives: %w", err)
	}
	for _, path := range sataMatches {
		drives = append(drives, Drive{
			Path: path,
			Name: filepath.Base(path),
			Type: DriveTypeSATA,
		})
	}

	nvmeMatches, err := filepath.Glob(filepath.Join(d.BasePath, "nvme[0-9]*n1"))
	if err != nil {
		return nil, fmt.Errorf("globbing NVMe drives: %w", err)
	}
	for _, path := range nvmeMatches {
		drives = append(drives, Drive{
			Path: path,
			Name: filepath.Base(path),
			Type: DriveTypeNVMe,
		})
	}

	sort.Slice(drives, func(i, j int) bool {
		return drives[i].Name < drives[j].Name
	})

	return drives, nil
}
