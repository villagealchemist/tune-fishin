// Package preflight compares a prepared set with a mounted rekordbox export.
package preflight

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/villagealchemist/tune-fishin/internal/legacy"
	"github.com/villagealchemist/tune-fishin/internal/library"
	"github.com/villagealchemist/tune-fishin/internal/onelibrary"
	"github.com/villagealchemist/tune-fishin/internal/volume"
)

const (
	deviceLibrary = "Device Library"
	oneLibrary    = "OneLibrary"
)

var Decks = map[string]string{
	"CDJ-3000X": oneLibrary, "CDJ-3000": deviceLibrary,
	"CDJ-3000 (firmware 3.30)": oneLibrary,
	"CDJ-2000NXS2":             deviceLibrary, "XDJ-XZ": deviceLibrary, "XDJ-RX3": deviceLibrary,
	"XDJ-AZ": oneLibrary, "OPUS-QUAD": oneLibrary, "OMNIS-DUO": oneLibrary,
}

type Config struct{ Source, USB, Deck, Playlist, Snapshot string }
type Report struct{ Status, Text string }

type readLibrary func(context.Context, string, string, string) ([]library.Playlist, error)
type inspectVolume func(context.Context, string, string) (*bool, string)

func Run(ctx context.Context, c Config) (Report, error) {
	return run(ctx, c, func(ctx context.Context, root, format, database string) ([]library.Playlist, error) {
		if format == oneLibrary {
			return onelibrary.Read(ctx, root, database)
		}
		return legacy.Read(ctx, database)
	}, volume.Inspect)
}

func run(ctx context.Context, c Config, read readLibrary, inspect inspectVolume) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	format, known := Decks[c.Deck]
	if !known || c.USB == "" || c.Source == "" {
		return Report{}, fmt.Errorf("choose a supported deck, a USB folder, and the intended M3U8 playlist")
	}
	root, err := canonical(c.USB)
	if err != nil {
		return Report{}, fmt.Errorf("open USB folder: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Report{}, err
	}
	if !info.IsDir() {
		return Report{}, fmt.Errorf("choose the root folder of the USB")
	}
	source, err := canonical(c.Source)
	if err != nil {
		return Report{}, fmt.Errorf("open intended playlist: %w", err)
	}
	expected, err := parseM3U(ctx, source, func(entry string) (string, error) {
		p, err := localPath(entry)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(source), p)
		}
		return p, nil
	})
	if err != nil {
		return Report{}, err
	}
	if len(expected) == 0 {
		return Report{}, fmt.Errorf("the intended playlist is empty")
	}
	playlist := c.Playlist
	if playlist == "" {
		base := filepath.Base(source)
		playlist = strings.TrimSuffix(base, filepath.Ext(base))
	}
	databaseName := "export.pdb"
	if format == oneLibrary {
		databaseName = "exportLibrary.db"
	}
	evidence := "USB library: " + databaseName
	compatible, volumeNote := inspect(ctx, root, c.Deck)
	report := func(status string, lines ...string) (Report, error) {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		if compatible != nil && !*compatible {
			status = "DIFFERENCES"
		} else if status == "MATCH" && compatible == nil {
			status = "INCOMPLETE"
		}
		all := []string{status + " | " + playlist, c.Deck + " · " + format, evidence, ""}
		all = append(all, lines...)
		if volumeNote != "" {
			all = append(all, "", volumeNote)
		}
		return Report{Status: status, Text: strings.Join(all, "\n")}, nil
	}
	dbReference := filepath.Join(root, "PIONEER", "rekordbox", databaseName)
	db, err := confined(root, dbReference)
	if err != nil {
		return report("INCOMPLETE", "Cannot resolve the USB library: "+err.Error())
	}
	initial, err := os.Stat(db)
	if errors.Is(err, fs.ErrNotExist) {
		return report("DIFFERENCES", fmt.Sprintf("The USB is missing %s (%s). Select the drive root or export the set to it in rekordbox.", format, databaseName))
	}
	if err != nil {
		return report("INCOMPLETE", "Cannot inspect the USB library: "+err.Error())
	}
	if !initial.Mode().IsRegular() {
		return report("INCOMPLETE", "The USB library path is not a regular file.")
	}
	playlists, readErr := read(ctx, root, format, db)
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	unchanged := func() bool {
		now, err := os.Stat(dbReference)
		return err == nil && sameVersion(initial, now)
	}
	if !unchanged() {
		return report("INCOMPLETE", "The USB library changed during the read. Finish exporting and run it again.")
	}
	var actual []track
	if readErr != nil {
		if c.Snapshot == "" {
			return report("INCOMPLETE", "Could not read "+format+": "+readErr.Error(), "In rekordbox, open Devices → this USB → "+format+" → Playlists. Export the device playlist as M3U8 and select it as the snapshot.")
		}
		snapshot, err := canonical(c.Snapshot)
		if err != nil {
			return Report{}, fmt.Errorf("open device snapshot: %w", err)
		}
		snapInfo, err := os.Stat(snapshot)
		if err != nil {
			return Report{}, err
		}
		srcInfo, err := os.Stat(source)
		if err != nil {
			return Report{}, err
		}
		if os.SameFile(srcInfo, snapInfo) {
			return report("INCOMPLETE", "Choose a separate playlist freshly exported from the USB library.")
		}
		evidence = "Device playlist snapshot supplied by you: " + filepath.Base(snapshot)
		actual, err = parseM3U(ctx, snapshot, func(entry string) (string, error) { return snapshotPath(root, snapshot, entry) })
		if err != nil {
			return Report{}, err
		}
	} else {
		var matches []library.Playlist
		for _, p := range playlists {
			if p.Name == playlist {
				matches = append(matches, p)
			}
		}
		if len(matches) == 0 {
			return report("DIFFERENCES", fmt.Sprintf("Playlist %q is missing from %s. Check the exact playlist name or export it to this USB.", playlist, format))
		}
		if len(matches) > 1 {
			return report("INCOMPLETE", fmt.Sprintf("%d playlists are named %q. Give the set a unique name and export it again.", len(matches), playlist))
		}
		for _, entry := range matches[0].Entries {
			p, err := devicePath(root, entry.Path)
			if err != nil {
				return report("INCOMPLETE", fmt.Sprintf("Invalid USB library reference for %q: %v", entry.Title, err))
			}
			name := entry.Title
			if name == "" {
				name = filepath.Base(p)
			}
			actual = append(actual, track{path: p, name: name})
		}
	}
	status, lines, err := compare(ctx, root, expected, actual)
	if err != nil {
		return Report{}, err
	}
	if !unchanged() {
		return report("INCOMPLETE", "The USB library changed during the check. Finish exporting and run it again.")
	}
	if readErr != nil && status == "MATCH" {
		status = "INCOMPLETE"
		lines = append(lines, "Order is from the supplied snapshot. The live device library could not be read.")
	}
	return report(status, lines...)
}
