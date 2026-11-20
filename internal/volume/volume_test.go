package volume

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatibility(t *testing.T) {
	for _, tc := range []struct{ fs, deck, want string }{
		{"MS-DOS FAT32", "CDJ-2000NXS2", "yes"},
		{"exFAT", "CDJ-2000NXS2", "no"},
		{"exFAT", "CDJ-3000X", "yes"},
		{"exFAT", "CDJ-3000", "1.20"},
		{"exFAT", "XDJ-XZ", "1.23"},
		{"exFAT", "XDJ-RX3", "1.11"},
		{"exFAT", "CDJ-3000 (firmware 3.30)", "yes"},
		{"NTFS", "XDJ-AZ", "no"},
		{"FAT16", "OPUS-QUAD", "yes"},
		{"HFS+", "OMNIS-DUO", "yes"},
		{"HFSX", "OMNIS-DUO", "unverified"},
		{"vfat", "XDJ-XZ", "unverified"},
		{"", "XDJ-RX3", "unverified"},
		{"FAT32", "example", "unverified"},
	} {
		value, message := classify(tc.fs, tc.deck)
		good := message != ""
		switch tc.want {
		case "yes":
			good = good && value != nil && *value
		case "no":
			good = good && value != nil && !*value
		case "unverified":
			good = good && value == nil
		default:
			good = good && value == nil && strings.Contains(message, tc.want)
		}
		if !good {
			t.Errorf("classify(%q,%q) = %v, %q; want %s", tc.fs, tc.deck, value, message, tc.want)
		}
	}
}

func TestBackupFolderIsUnverified(t *testing.T) {
	root := t.TempDir()
	audio := filepath.Join(root, "track.mp3")
	if err := os.WriteFile(audio, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, audio, filepath.Join(root, "missing")} {
		if value, _ := Inspect(context.Background(), path, "CDJ-3000"); value != nil {
			t.Errorf("reported a mounted filesystem for %s", path)
		}
	}
}

func TestMacMount(t *testing.T) {
	info := `<plist><dict><key>MountPoint</key><string>/Volumes/GIG</string><key>Internal</key><false/>
		<key>FilesystemType</key><string>msdos</string><key>FilesystemName</key><string>MS-DOS FAT32</string>
		<key>Other</key><dict><key>MountPoint</key><string>/Volumes/other</string></dict></dict></plist>`
	if got := macFilesystem("/Volumes/GIG", []byte(info)); got != "MS-DOS FAT32" {
		t.Errorf("mounted stick: %q", got)
	}
	for _, test := range []struct{ root, data string }{
		{"/Volumes/GIG/Backup", info},
		{"/Volumes/GIG", strings.Replace(info, "<false/>", "<true/>", 1)},
		{"/Volumes/GIG", strings.Replace(info, "<key>Other", "<key>BusProtocol</key><string>Disk Image</string><key>Other", 1)},
		{"/Volumes/GIG", "<plist>"},
	} {
		if got := macFilesystem(test.root, []byte(test.data)); got != "" {
			t.Errorf("unverified volume: %q", got)
		}
	}
}

func TestLinuxMount(t *testing.T) {
	mounts := `{"filesystems":[{"target":"/media/usb","source":"/dev/sdb1","fstype":"vfat","fsroot":"/"}]}`
	disks := `{"blockdevices":[{"name":"sdb1","fsver":"FAT32","rm":false,"children":[{"name":"sdb","rm":false,"tran":"usb"}]}]}`
	for _, test := range []struct{ root, mounts, disks, want string }{
		{"/media/usb", mounts, disks, "FAT32"},
		{"/media/usb", mounts, strings.Replace(disks, "FAT32", "", 1), "vfat"},
		{"/media/usb/Backup", mounts, disks, ""},
		{"/media/usb", strings.Replace(mounts, `"fsroot":"/"`, `"fsroot":"/sub"`, 1), disks, ""},
		{"/media/usb", mounts, strings.Replace(disks, "usb", "sata", 1), ""},
		{"/media/usb", mounts, `{"blockdevices":[{"rm":"1","fsver":"FAT16"}]}`, "FAT16"},
	} {
		run := func(command string, args ...string) ([]byte, error) {
			switch command {
			case "findmnt":
				return []byte(test.mounts), nil
			case "lsblk":
				return []byte(test.disks), nil
			}
			return nil, errors.New("unexpected command")
		}
		if got := linuxFilesystem(test.root, run); got != test.want {
			t.Errorf("linuxFilesystem(%q, %q, %q) = %q, want %q", test.root, test.mounts, test.disks, got, test.want)
		}
	}
}
