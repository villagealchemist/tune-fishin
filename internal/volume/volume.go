// Package volume checks filesystem compatibility on a mounted external device.
package volume

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const unverified = "Filesystem unverified. Select the mounted USB volume itself to check its format."

// Inspect reports compatibility of the filesystem alone. A nil result means the
// selected path, filesystem variant or venue firmware needs confirmation.
func Inspect(ctx context.Context, root, deck string) (*bool, string) {
	if ctx == nil || ctx.Err() != nil || root == "" {
		return nil, unverified
	}
	path, err := filepath.Abs(root)
	if err != nil {
		return nil, unverified
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, unverified
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() || (runtime.GOOS != "windows" && path == "/") {
		return nil, unverified
	}
	probe, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	run := func(command string, args ...string) ([]byte, error) {
		return exec.CommandContext(probe, command, args...).Output()
	}
	var fs string
	switch runtime.GOOS {
	case "darwin":
		out, err := run("/usr/sbin/diskutil", "info", "-plist", path)
		if err == nil {
			fs = macFilesystem(path, out)
		}
	case "linux":
		fs = linuxFilesystem(path, run)
	case "windows":
		// A removable stick and a USB SSD can report different drive types.
		// Get-Disk's physical bus also distinguishes fixed USB media from internal disks.
		drive := filepath.VolumeName(path)
		if len(drive) == 2 && drive[1] == ':' && strings.EqualFold(path, drive+`\`) {
			letter := strings.ToUpper(drive[:1])
			if letter[0] >= 'A' && letter[0] <= 'Z' {
				script := fmt.Sprintf(`$ErrorActionPreference='Stop'; $disk=Get-Partition -DriveLetter %s | Get-Disk; if (([string]$disk.BusType) -in @('USB','SD','MMC','1394')) { (Get-Volume -DriveLetter %s).FileSystem }`, letter, letter)
				out, err := run("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
				if err == nil {
					fs = strings.TrimSpace(string(out))
				}
			}
		}
	}
	return classify(fs, deck)
}

// diskutil info -plist returns a dictionary. Decode its top-level scalar fields
// and skip any nested data, which can contain unrelated keys of the same name.
func macFilesystem(root string, data []byte) string {
	type dictionary struct {
		Elements []struct {
			XMLName xml.Name
			Text    string `xml:",chardata"`
		} `xml:",any"`
	}
	var plist struct {
		XMLName xml.Name   `xml:"plist"`
		Dict    dictionary `xml:"dict"`
	}
	if xml.Unmarshal(data, &plist) != nil {
		return ""
	}
	fields := map[string]string{}
	for i := 0; i+1 < len(plist.Dict.Elements); i += 2 {
		key, value := plist.Dict.Elements[i], plist.Dict.Elements[i+1]
		if key.XMLName.Local != "key" {
			return ""
		}
		switch value.XMLName.Local {
		case "string":
			fields[key.Text] = value.Text
		case "true", "false":
			fields[key.Text] = value.XMLName.Local
		}
	}
	bus := strings.ToLower(fields["BusProtocol"])
	if fields["MountPoint"] != root || bus == "disk image" || strings.EqualFold(fields["VirtualOrPhysical"], "virtual") {
		return ""
	}
	if fields["Internal"] != "false" && fields["RemovableMedia"] != "true" && bus != "usb" {
		return ""
	}
	if fs := fields["FilesystemName"]; fs != "" {
		return fs
	}
	return fields["FilesystemType"]
}

type blockDevice struct {
	Transport string          `json:"tran"`
	Removable json.RawMessage `json:"rm"`
	FSVersion string          `json:"fsver"`
	Children  []blockDevice   `json:"children"`
}

func (device blockDevice) external() bool {
	switch device.Transport {
	case "usb", "mmc", "ieee1394":
		return true
	}
	switch strings.Trim(string(device.Removable), `"`) {
	case "true", "1":
		return true
	}
	for _, parent := range device.Children {
		if parent.external() {
			return true
		}
	}
	return false
}

func linuxFilesystem(root string, run func(string, ...string) ([]byte, error)) string {
	out, err := run("findmnt", "--json", "--mountpoint", root, "--output", "TARGET,SOURCE,FSTYPE,FSROOT")
	var mounts struct {
		Filesystems []struct{ Target, Source, FSType, FSRoot string }
	}
	if err != nil || json.Unmarshal(out, &mounts) != nil || len(mounts.Filesystems) != 1 {
		return ""
	}
	mount := mounts.Filesystems[0]
	if filepath.Clean(mount.Target) != root || mount.FSRoot != "/" || !strings.HasPrefix(mount.Source, "/dev/") {
		return ""
	}
	// Inverse mode walks from a partition to its parent disk. Many USB SSDs report rm=0.
	out, err = run("lsblk", "--inverse", "--tree", "--json", "--output", "NAME,TRAN,RM,FSVER", "--", mount.Source)
	var disks struct{ Blockdevices []blockDevice }
	if err != nil || json.Unmarshal(out, &disks) != nil || len(disks.Blockdevices) != 1 || !disks.Blockdevices[0].external() {
		return ""
	}
	if mount.FSType == "vfat" || mount.FSType == "msdos" {
		switch disks.Blockdevices[0].FSVersion {
		case "FAT12", "FAT16", "FAT32":
			return disks.Blockdevices[0].FSVersion
		}
	}
	return mount.FSType
}

func known(supported bool, message string) (*bool, string) { return &supported, message }

// AlphaTheta lists model-specific USB formats in the OneLibrary Compatible USB
// Device Export Guide and gives exFAT firmware thresholds in rekordbox 6.6.2 release notes.
func classify(raw, deck string) (*bool, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, unverified
	}
	deck = strings.ToUpper(strings.TrimSpace(deck))
	firmware := ""
	switch deck {
	case "CDJ-3000":
		firmware = "1.20"
	case "XDJ-XZ":
		firmware = "1.23"
	case "XDJ-RX3":
		firmware = "1.11"
	case "CDJ-3000 (FIRMWARE 3.30)":
		deck = "CDJ-3000 (firmware 3.30)"
	case "CDJ-3000X", "XDJ-AZ", "OPUS-QUAD", "OMNIS-DUO", "CDJ-2000NXS2":
	default:
		return nil, fmt.Sprintf("Detected %s. Filesystem compatibility is unknown for %s.", raw, deck)
	}
	switch strings.ToLower(raw) {
	case "fat32", "ms-dos fat32", "fat16", "ms-dos fat16",
		"hfs+", "journaled hfs+", "mac os extended", "mac os extended (journaled)":
		return known(true, fmt.Sprintf("%s is a supported filesystem on the %s.", raw, deck))
	case "exfat":
		if deck == "CDJ-2000NXS2" {
			return known(false, "The CDJ-2000NXS2 cannot read exFAT.")
		}
		if firmware != "" {
			return nil, fmt.Sprintf("exFAT requires %s firmware %s or newer. Check the venue's deck.", deck, firmware)
		}
		return known(true, fmt.Sprintf("exFAT is a supported filesystem on the %s.", deck))
	case "ntfs", "ntfs3", "apfs", "ext2", "ext3", "ext4", "xfs", "btrfs", "fat12", "ms-dos fat12", "refs":
		return known(false, fmt.Sprintf("The %s does not support the %s filesystem.", deck, raw))
	}
	return nil, fmt.Sprintf("Detected %s. The exact supported filesystem could not be verified.", raw)
}
