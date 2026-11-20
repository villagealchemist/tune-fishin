// Package onelibrary reads the optional OneLibrary export through a private local snapshot.
package onelibrary

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/villagealchemist/tune-fishin/internal/library"
)

//go:embed adapter.py
var adapter string

const maxRecord = 1 << 20

var errChanged = errors.New("OneLibrary changed during the check; finish exporting and retry")
var errReader = errors.New("direct OneLibrary checks need Python 3.11+ with pyrekordbox==0.5.0 and sqlcipher3-wheels; set TUNA_PYTHON to that interpreter or supply a fresh device playlist snapshot")

// Read returns ordinary playlists in the order recorded on the device, including repeated tracks.
func Read(ctx context.Context, usbRoot, filename string) ([]library.Playlist, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := checkSource(filename)
	if err != nil {
		return nil, err
	}
	python, err := findPython()
	if err != nil {
		return nil, err
	}
	dir, err := privateDir(usbRoot)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	copyPath := filepath.Join(dir, "exportLibrary.db")
	dst, err := os.OpenFile(copyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	expected := sha256.New()
	err = readStable(ctx, filename, before, io.MultiWriter(dst, expected))
	closeErr := dst.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	playlists, err := runAdapter(ctx, python, dir)
	if err != nil {
		return nil, err
	}
	// FAT timestamps can miss same-size rewrites. Compare bytes before accepting the snapshot.
	actual := sha256.New()
	if err := readStable(ctx, filename, before, actual); err != nil {
		return nil, err
	}
	if !bytes.Equal(expected.Sum(nil), actual.Sum(nil)) {
		return nil, errChanged
	}
	return playlists, nil
}

func checkSource(filename string) (os.FileInfo, error) {
	for _, suffix := range []string{"-wal", "-journal", "-shm"} {
		info, err := os.Lstat(filename + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		// A shared-memory index can be left behind on a checkpointed, closed
		// export. A regular shm alone contains no outstanding transactions.
		if !info.Mode().IsRegular() || (suffix != "-shm" && info.Size() != 0) {
			return nil, errors.New("OneLibrary has an active or unreadable journal; finish exporting, safely eject and reconnect the drive")
		}
	}
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, errors.New("OneLibrary database is empty or is not a regular file")
	}
	return info, nil
}

func findPython() (string, error) {
	candidates := []string{"python3"}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, "python")
	}
	if override := os.Getenv("TUNA_PYTHON"); override != "" {
		candidates = []string{override}
	}
	for _, candidate := range candidates {
		name, err := exec.LookPath(candidate)
		if err == nil {
			return filepath.Abs(name)
		}
	}
	return "", errReader
}

func privateDir(usbRoot string) (string, error) {
	// Compare against the user-selected volume, even if the library lives
	// beneath a symlink to a differently named directory on that volume.
	root, err := filepath.EvalSymlinks(usbRoot)
	if err != nil {
		return "", err
	}
	device, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !device.IsDir() {
		return "", errors.New("selected USB root must be a directory")
	}
	tmp, err := filepath.Abs(os.TempDir())
	if err != nil {
		return "", err
	}
	tmp, err = filepath.EvalSymlinks(tmp)
	if err != nil {
		return "", err
	}
	for dir := tmp; ; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err != nil {
			return "", err
		}
		if os.SameFile(device, info) {
			return "", errors.New("the temporary directory points inside the USB; choose a temporary directory on the computer")
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return os.MkdirTemp(tmp, "tune-fishin-")
}

func sameExport(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

type checkedReader struct {
	ctx context.Context
	r   io.Reader
}

func (r checkedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func readStable(ctx context.Context, filename string, expected os.FileInfo, dst io.Writer) error {
	current, err := checkSource(filename)
	if err != nil {
		return err
	}
	if !sameExport(expected, current) {
		return errChanged
	}
	src, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer src.Close()
	opened, err := src.Stat()
	if err != nil {
		return err
	}
	if !sameExport(expected, opened) {
		return errChanged
	}
	n, err := io.Copy(dst, io.LimitReader(checkedReader{ctx, src}, expected.Size()+1))
	if err != nil {
		return err
	}
	current, err = checkSource(filename)
	if err != nil {
		return err
	}
	if n != expected.Size() || !sameExport(expected, current) {
		return errChanged
	}
	return nil
}

func runAdapter(ctx context.Context, python, dir string) ([]library.Playlist, error) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(child, python, "-B", "-c", adapter)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second

	// Close the read side on a malformed record so the writer can exit, even
	// when it has already filled its stdout pipe. Always wait for the process.
	stream, output := io.Pipe()
	cmd.Stdout = output
	finished := make(chan error, 1)
	go func() {
		err := cmd.Run()
		output.Close()
		finished <- err
	}()
	playlists, parseErr := decode(stream)
	stream.Close()
	if parseErr != nil {
		cancel()
	}
	err := <-finished
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 20 {
		return nil, errReader
	}
	if err != nil && (parseErr == nil || errors.Is(parseErr, io.ErrUnexpectedEOF)) {
		return nil, errors.New("the optional OneLibrary reader could not read this export; check the on-device playlist in rekordbox or supply a fresh device playlist snapshot")
	}
	return playlists, parseErr
}

func decode(stream io.Reader) ([]library.Playlist, error) {
	var playlists []library.Playlist
	rows := bufio.NewScanner(stream)
	rows.Buffer(make([]byte, 4096), maxRecord)
	var previous int64 = -1
	done := false
	for rows.Scan() {
		var row struct {
			Name     *string `json:"name"`
			Position *int64  `json:"position"`
			Path     *string `json:"path"`
			Title    string  `json:"title"`
			Done     bool    `json:"done"`
		}
		if err := json.Unmarshal(rows.Bytes(), &row); err != nil || done {
			return nil, errors.New("invalid output from the OneLibrary reader")
		}
		switch {
		case row.Done:
			done = true
		case row.Name != nil && *row.Name != "":
			playlists = append(playlists, library.Playlist{Name: *row.Name})
			previous = -1
		default:
			if len(playlists) == 0 || row.Position == nil || *row.Position <= previous {
				return nil, errors.New("OneLibrary contains an unnamed playlist or ambiguous track order")
			}
			list := &playlists[len(playlists)-1]
			if row.Path == nil || *row.Path == "" || strings.ContainsRune(*row.Path, 0) {
				return nil, fmt.Errorf("OneLibrary has a missing track reference in %q", list.Name)
			}
			title := row.Title
			if title == "" {
				title = path.Base(*row.Path)
			}
			list.Entries = append(list.Entries, library.Entry{Path: *row.Path, Title: title})
			previous = *row.Position
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("OneLibrary reader record exceeds 1 MiB or could not be read: %w", err)
	}
	if !done {
		return nil, fmt.Errorf("incomplete output from the OneLibrary reader: %w", io.ErrUnexpectedEOF)
	}
	return playlists, nil
}
