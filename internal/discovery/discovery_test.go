package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func createFakeDevices(t *testing.T, dir string, names []string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscovery_SATAOnly(t *testing.T) {
	dir := t.TempDir()
	createFakeDevices(t, dir, []string{"sata1", "sata2", "sata3"})

	d := NewDevFSDiscoverer(dir)
	drives, err := d.Discover()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drives) != 3 {
		t.Fatalf("got %d drives, want 3", len(drives))
	}
	for _, drive := range drives {
		if drive.Type != DriveTypeSATA {
			t.Errorf("drive %s type = %s, want sata", drive.Name, drive.Type)
		}
	}
}

func TestDiscovery_NVMeOnly(t *testing.T) {
	dir := t.TempDir()
	createFakeDevices(t, dir, []string{"nvme0n1", "nvme1n1"})

	d := NewDevFSDiscoverer(dir)
	drives, err := d.Discover()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drives) != 2 {
		t.Fatalf("got %d drives, want 2", len(drives))
	}
	for _, drive := range drives {
		if drive.Type != DriveTypeNVMe {
			t.Errorf("drive %s type = %s, want nvme", drive.Name, drive.Type)
		}
	}
}

func TestDiscovery_Mixed(t *testing.T) {
	dir := t.TempDir()
	createFakeDevices(t, dir, []string{"sata1", "sata2", "nvme0n1"})

	d := NewDevFSDiscoverer(dir)
	drives, err := d.Discover()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drives) != 3 {
		t.Fatalf("got %d drives, want 3", len(drives))
	}

	sataCount := 0
	nvmeCount := 0
	for _, drive := range drives {
		switch drive.Type {
		case DriveTypeSATA:
			sataCount++
		case DriveTypeNVMe:
			nvmeCount++
		}
	}
	if sataCount != 2 {
		t.Errorf("sata count = %d, want 2", sataCount)
	}
	if nvmeCount != 1 {
		t.Errorf("nvme count = %d, want 1", nvmeCount)
	}
}

func TestDiscovery_Empty(t *testing.T) {
	dir := t.TempDir()

	d := NewDevFSDiscoverer(dir)
	drives, err := d.Discover()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drives) != 0 {
		t.Errorf("got %d drives, want 0", len(drives))
	}
}

func TestDiscovery_Sorting(t *testing.T) {
	dir := t.TempDir()
	createFakeDevices(t, dir, []string{"sata3", "sata1", "nvme0n1", "sata2"})

	d := NewDevFSDiscoverer(dir)
	drives, err := d.Discover()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"nvme0n1", "sata1", "sata2", "sata3"}
	if len(drives) != len(expected) {
		t.Fatalf("got %d drives, want %d", len(drives), len(expected))
	}
	for i, drive := range drives {
		if drive.Name != expected[i] {
			t.Errorf("drive[%d] = %q, want %q", i, drive.Name, expected[i])
		}
	}
}

func TestDiscovery_IgnoresNonMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	createFakeDevices(t, dir, []string{"sata1", "sda", "nvme0n1", "nvme0n1p1", "ttyS0"})

	d := NewDevFSDiscoverer(dir)
	drives, err := d.Discover()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drives) != 2 {
		t.Fatalf("got %d drives, want 2: %+v", len(drives), drives)
	}
}

func TestDiscovery_DefaultBasePath(t *testing.T) {
	d := NewDevFSDiscoverer("")
	if d.BasePath != "/dev" {
		t.Errorf("basePath = %q, want /dev", d.BasePath)
	}
}
