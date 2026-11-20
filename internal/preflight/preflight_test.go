package preflight

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/villagealchemist/tune-fishin/internal/library"
)

type fixture struct {
	root       string
	config     Config
	playlists  []library.Playlist
	dbError    error
	compatible *bool
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	usb := filepath.Join(root, "Gig USB")
	source := filepath.Join(root, "Studio", "Friday.m3u8")
	f := &fixture{root: root, config: Config{USB: usb, Source: source, Deck: "CDJ-2000NXS2"}, compatible: new(bool)}
	*f.compatible = true
	f.playlists = []library.Playlist{{Name: "Friday", Entries: []library.Entry{{Path: "/Contents/Opener.wav", Title: "Opener"}, {Path: "/Contents/Closer.wav", Title: "Closer"}}}}
	for _, name := range []string{"Opener", "Closer"} {
		write(t, filepath.Join(usb, "Contents", name+".wav"), name)
		write(t, filepath.Join(filepath.Dir(source), name+".wav"), name)
	}
	write(t, source, "\uFEFF#EXTM3U\r\nOpener.wav\r\nCloser.wav\r\n")
	write(t, filepath.Join(usb, "PIONEER", "rekordbox", "export.pdb"), "record")
	return f
}

func (f *fixture) check(t *testing.T, want, part string) {
	t.Helper()
	r, err := run(context.Background(), f.config, func(context.Context, string, string, string) ([]library.Playlist, error) {
		return f.playlists, f.dbError
	}, func(context.Context, string, string) (*bool, string) { return f.compatible, "Volume under test" })
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != want || !strings.Contains(r.Text, part) {
		t.Fatalf("wanted %s and %q; got %s", want, part, r.Text)
	}
}

func TestReconciliation(t *testing.T) {
	cases := []struct {
		name, status, detail string
		change               func(*testing.T, *fixture)
	}{
		{"matching set", "MATCH", "in order", nil},
		{"unknown volume", "INCOMPLETE", "in order", func(t *testing.T, f *fixture) { f.compatible = nil }},
		{"incompatible volume", "DIFFERENCES", "Volume under test", func(t *testing.T, f *fixture) { *f.compatible = false }},
		{"reordered", "DIFFERENCES", "Order:", func(t *testing.T, f *fixture) { es := f.playlists[0].Entries; es[0], es[1] = es[1], es[0] }},
		{"duplicate occurrence omitted", "DIFFERENCES", "Missing from playlist at 3", func(t *testing.T, f *fixture) { write(t, f.config.Source, "Opener.wav\nCloser.wav\nOpener.wav\n") }},
		{"only stored outside playlist", "DIFFERENCES", "identical audio found on USB", func(t *testing.T, f *fixture) { f.playlists[0].Entries = f.playlists[0].Entries[:1] }},
		{"playlist absent", "DIFFERENCES", "missing from Device Library", func(t *testing.T, f *fixture) { f.playlists[0].Name = "Old set" }},
		{"name collision", "INCOMPLETE", "2 playlists", func(t *testing.T, f *fixture) { f.playlists = append(f.playlists, f.playlists[0]) }},
		{"same name and length changed", "DIFFERENCES", "no identical file", func(t *testing.T, f *fixture) {
			write(t, filepath.Join(f.config.USB, "Contents", "Closer.wav"), "clOSer")
		}},
		{"missing USB file", "INCOMPLETE", "USB track 2", func(t *testing.T, f *fixture) {
			if err := os.Remove(filepath.Join(f.config.USB, "Contents", "Closer.wav")); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing source file", "INCOMPLETE", "Intended track 2", func(t *testing.T, f *fixture) {
			if err := os.Remove(filepath.Join(filepath.Dir(f.config.Source), "Closer.wav")); err != nil {
				t.Fatal(err)
			}
		}},
		{"invalid catalog", "INCOMPLETE", "Could not read Device Library", func(t *testing.T, f *fixture) { f.dbError = errors.New("unreadable test record") }},
		{"catalog traversal", "INCOMPLETE", "Invalid USB library reference", func(t *testing.T, f *fixture) { f.playlists[0].Entries[0].Path = "../Studio/Opener.wav" }},
		{"symlink to laptop", "INCOMPLETE", "outside", func(t *testing.T, f *fixture) {
			link := filepath.Join(f.config.USB, "Contents", "Closer.wav")
			if err := os.Remove(link); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(filepath.Dir(f.config.Source), "Closer.wav"), link); err != nil {
				t.Skip(err)
			}
		}},
		{"requested catalog absent", "DIFFERENCES", "missing OneLibrary", func(t *testing.T, f *fixture) { f.config.Deck = "CDJ-3000X" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			if tc.change != nil {
				tc.change(t, f)
			}
			f.check(t, tc.status, tc.detail)
		})
	}
}

func TestSnapshotEvidence(t *testing.T) {
	for _, tc := range []struct{ name, db, body, status, detail string }{
		{"matching snapshot", "unreadable", "/Contents/Opener.wav\n/Contents/Closer.wav\n", "INCOMPLETE", "supplied snapshot"},
		{"reordered snapshot", "unreadable", "/Contents/Closer.wav\n/Contents/Opener.wav\n", "DIFFERENCES", "Order:"},
		{"live library takes precedence", "", "/Contents/Closer.wav\n", "MATCH", "USB library: export.pdb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			if tc.db != "" {
				f.dbError = errors.New(tc.db)
			}
			f.config.Snapshot = filepath.Join(f.root, "snapshot.m3u8")
			write(t, f.config.Snapshot, tc.body)
			f.check(t, tc.status, tc.detail)
		})
	}
	t.Run("same file alias", func(t *testing.T) {
		f := setup(t)
		f.dbError = errors.New("unreadable")
		f.config.Snapshot = filepath.Join(f.root, "copy.m3u8")
		if err := os.Link(f.config.Source, f.config.Snapshot); err != nil {
			t.Skip(err)
		}
		f.check(t, "INCOMPLETE", "separate")
	})
}

func TestUnconfinedSnapshotAndCancellation(t *testing.T) {
	f := setup(t)
	f.config.Snapshot = filepath.Join(f.root, "snapshot.m3u8")
	write(t, f.config.Snapshot, filepath.Join(filepath.Dir(f.config.Source), "Opener.wav")+"\n")
	read := func(context.Context, string, string, string) ([]library.Playlist, error) {
		return nil, errors.New("unreadable")
	}
	inspect := func(context.Context, string, string) (*bool, string) { return f.compatible, "" }
	if _, err := run(context.Background(), f.config, read, inspect); err == nil {
		t.Fatal("accepted snapshot pointing outside the USB")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, f.config); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestDatabaseChangeDuringRead(t *testing.T) {
	f := setup(t)
	read := func(_ context.Context, _, _, path string) ([]library.Playlist, error) {
		write(t, path, "replacement catalog")
		return f.playlists, nil
	}
	report, err := run(context.Background(), f.config, read, func(context.Context, string, string) (*bool, string) { return f.compatible, "" })
	if err != nil || report.Status != "INCOMPLETE" || !strings.Contains(report.Text, "changed") {
		t.Fatalf("%v: %s", err, report.Text)
	}
}
