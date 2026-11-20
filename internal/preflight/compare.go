package preflight

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type signature [sha256.Size]byte
type track struct {
	name, path string
	digest     signature
	size       int64
}
type fileHash struct {
	info   fs.FileInfo
	digest signature
}
type cache map[string]fileHash

func sameVersion(before, after fs.FileInfo) bool {
	return before != nil && after != nil && os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

// Repeated entries may reuse their hash only while the file version matches.
func (c cache) fingerprint(ctx context.Context, path string) (fileHash, error) {
	if err := ctx.Err(); err != nil {
		return fileHash{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fileHash{}, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fileHash{}, fmt.Errorf("empty or not a regular file: %s", path)
	}
	if previous, ok := c[path]; ok && sameVersion(previous.info, info) {
		return previous, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return fileHash{}, err
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, contextReader{ctx, f})
	if err != nil {
		return fileHash{}, err
	}
	after, err := f.Stat()
	if err != nil {
		return fileHash{}, err
	}
	if !sameVersion(info, after) {
		return fileHash{}, fmt.Errorf("file changed while checking: %s", path)
	}
	var result fileHash
	result.info = after
	h.Sum(result.digest[:0])
	c[path] = result
	return result, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func (c cache) identify(ctx context.Context, tracks []track, label string) []string {
	var issues []string
	for i := range tracks {
		t := &tracks[i]
		h, err := c.fingerprint(ctx, t.path)
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s track %d, %s: %v", label, i+1, t.name, err))
			if ctx.Err() != nil {
				return issues
			}
			continue
		}
		t.digest, t.size = h.digest, h.info.Size()
	}
	return issues
}

func findCopies(ctx context.Context, root string, missing []int, expected []track, hashes cache) (map[signature]bool, bool) {
	wanted := map[signature]bool{}
	sizes := map[int64]bool{}
	for _, i := range missing {
		wanted[expected[i].digest] = true
		sizes[expected[i].size] = true
	}
	found := map[signature]bool{}
	complete := true
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return fs.SkipAll
		}
		if err != nil {
			complete = false
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			complete = false
			return nil
		}
		if !sizes[info.Size()] {
			return nil
		}
		h, err := hashes.fingerprint(ctx, path)
		if err != nil {
			complete = false
			return nil
		}
		if wanted[h.digest] {
			found[h.digest] = true
		}
		if len(found) == len(wanted) {
			return fs.SkipAll
		}
		return nil
	})
	return found, complete
}

func compare(ctx context.Context, root string, expected, actual []track) (string, []string, error) {
	hashes := cache{}
	lines := []string{fmt.Sprintf("%d intended tracks · %d exported entries", len(expected), len(actual))}
	lines = append(lines, hashes.identify(ctx, expected, "Intended")...)
	lines = append(lines, hashes.identify(ctx, actual, "USB")...)
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if len(lines) > 1 {
		return "INCOMPLETE", append(lines, "Reconnect the source audio and re-export any missing or empty USB files, then check again."), nil
	}

	positions := map[signature][]int{}
	for i, t := range actual {
		positions[t.digest] = append(positions[t.digest], i)
	}
	// Consume occurrences in playlist order, including repeated identical audio.
	type match struct{ source, destination int }
	var matched []match
	var missing []int
	used := make([]bool, len(actual))
	reordered := false
	for i, t := range expected {
		queue := positions[t.digest]
		if len(queue) == 0 {
			missing = append(missing, i)
			continue
		}
		j := queue[0]
		positions[t.digest] = queue[1:]
		if len(matched) > 0 && j < matched[len(matched)-1].destination {
			reordered = true
		}
		matched = append(matched, match{i, j})
		used[j] = true
	}
	if len(missing) > 0 {
		found, complete := findCopies(ctx, root, missing, expected, hashes)
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		if !complete {
			lines = append(lines, "Some USB files could not be searched for matching audio.")
		}
		for _, i := range missing {
			t := expected[i]
			detail := "no identical file found in the readable USB files"
			if found[t.digest] {
				detail = "identical audio found on USB"
			}
			lines = append(lines, fmt.Sprintf("Missing from playlist at %d: %s (%s).", i+1, t.name, detail))
		}
	}
	for j, wasUsed := range used {
		if !wasUsed {
			lines = append(lines, fmt.Sprintf("Unexpected USB track at %d: %s.", j+1, actual[j].name))
		}
	}
	if reordered {
		for _, m := range matched {
			if m.source != m.destination {
				lines = append(lines, fmt.Sprintf("Order: %s is at %d on USB, intended at %d.", expected[m.source].name, m.destination+1, m.source+1))
			}
		}
	}
	if len(missing) > 0 || len(actual) != len(matched) || reordered {
		return "DIFFERENCES", append(lines, "Reconcile the playlist in rekordbox, export it again, and recheck."), nil
	}
	// An early track can change while a later track is hashing. Refresh every
	// selected file's identity once before producing a successful comparison.
	for _, group := range [][]track{expected, actual} {
		for _, t := range group {
			if err := ctx.Err(); err != nil {
				return "", nil, err
			}
			fi, err := os.Stat(t.path)
			if err != nil || !sameVersion(hashes[t.path].info, fi) {
				return "INCOMPLETE", append(lines, "A file changed during the check: "+t.path+". Run the check again."), nil
			}
		}
	}
	return "MATCH", append(lines, "Every playlist entry matches the intended audio file byte for byte, in order."), nil
}
