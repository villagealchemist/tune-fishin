package preflight

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

func canonical(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

func below(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// Resolve every existing ancestor before trusting a path. Missing leaves may
// still be reported separately as a missing database or missing track.
func confined(root, path string) (string, error) {
	if !below(root, path) {
		return "", fmt.Errorf("path points outside the selected USB: %s", path)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	current := root
	parts := strings.Split(relative, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		entry, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return filepath.Join(append([]string{current}, parts[i+1:]...)...), nil
		}
		if err != nil {
			return "", err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			current, err = filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("unresolved USB link: %v", err)
			}
			if !below(root, current) {
				return "", fmt.Errorf("USB link points outside the drive: %s", current)
			}
		}
	}
	return current, nil
}

func windowsDrive(p string) bool {
	return len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))
}

func localPath(entry string) (string, error) {
	if strings.IndexByte(entry, 0) >= 0 {
		return "", fmt.Errorf("playlist path contains a NUL byte")
	}
	if strings.HasPrefix(strings.ToLower(entry), "file:") {
		u, err := url.Parse(entry)
		if err != nil {
			return "", err
		}
		if (u.Host != "" && !strings.EqualFold(u.Host, "localhost")) || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.Path == "" || !strings.HasPrefix(u.Path, "/") {
			return "", fmt.Errorf("unsupported file URL: %s", entry)
		}
		entry = u.Path
		if runtime.GOOS == "windows" && len(entry) > 1 && entry[0] == '/' && windowsDrive(entry[1:]) {
			entry = entry[1:]
		}
	}
	if strings.Contains(entry, "://") || strings.HasPrefix(strings.ReplaceAll(entry, "\\", "/"), "//") {
		return "", fmt.Errorf("this check needs a local audio file: %s", entry)
	}
	if windowsDrive(entry) && (runtime.GOOS != "windows" || !filepath.IsAbs(entry)) {
		return "", fmt.Errorf("this path requires its original Windows volume: %s", entry)
	}
	if strings.IndexByte(entry, 0) >= 0 {
		return "", fmt.Errorf("file URL contains a NUL byte")
	}
	return filepath.FromSlash(entry), nil
}

func devicePath(root, entry string) (string, error) {
	p := strings.ReplaceAll(entry, "\\", "/")
	if p == "" || strings.Contains("/"+p+"/", "/../") || strings.Contains(p, "://") || strings.HasPrefix(p, "//") || strings.IndexByte(p, 0) >= 0 || windowsDrive(strings.TrimLeft(p, "/")) {
		return "", fmt.Errorf("invalid device track path: %q", entry)
	}
	return confined(root, filepath.Join(root, filepath.FromSlash(strings.TrimLeft(p, "/"))))
}

func snapshotPath(root, snapshot, entry string) (string, error) {
	p, err := localPath(entry)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(p) && below(root, p) {
		return confined(root, p)
	}
	if strings.HasPrefix(filepath.ToSlash(p), "/Contents/") {
		return devicePath(root, p)
	}
	if filepath.IsAbs(p) {
		// macOS can expose the same volume under a canonical path and an alias.
		if resolved, err := filepath.EvalSymlinks(p); err == nil && below(root, resolved) {
			return resolved, nil
		}
		return "", fmt.Errorf("snapshot path points outside the USB: %s", p)
	}
	return confined(root, filepath.Join(filepath.Dir(snapshot), p))
}

func parseM3U(ctx context.Context, path string, resolve func(string) (string, error)) ([]track, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".m3u8" && ext != ".m3u" {
		return nil, fmt.Errorf("choose a UTF-8 M3U or M3U8 playlist: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("playlist is not a regular file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	var tracks []track
	for n := 1; scan.Scan(); n++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := scan.Text()
		if n == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}
		if !utf8.ValidString(line) {
			return nil, fmt.Errorf("%s, line %d: invalid UTF-8", path, n)
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p, err := resolve(line)
		if err != nil {
			return nil, fmt.Errorf("%s, line %d: %w", path, n, err)
		}
		tracks = append(tracks, track{name: filepath.Base(p), path: p})
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("read playlist %s: %w", path, err)
	}
	return tracks, nil
}
