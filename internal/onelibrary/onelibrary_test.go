package onelibrary

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/villagealchemist/tune-fishin/internal/library"
)

func TestDecodePreservesPlaylistOccurrences(t *testing.T) {
	got, err := decode([]byte(`{"name":"Set"}
{"position":1,"path":"/Contents/a.aif","title":"Opener"}
{"position":2,"path":"/Contents/a.aif","title":"Opener"}
{"name":"Set"}
{"name":"Encore"}
{"position":1,"path":"/Contents/b.aif","title":null}
{"done":true}`))
	track := library.Entry{Path: "/Contents/a.aif", Title: "Opener"}
	want := []library.Playlist{
		{Name: "Set", Entries: []library.Entry{track, track}}, {Name: "Set"},
		{Name: "Encore", Entries: []library.Entry{{Path: "/Contents/b.aif", Title: "b.aif"}}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestRejectIncompleteProtocol(t *testing.T) {
	for _, input := range []string{
		"", `{"name":"X"}`, `{"position":1,"path":"abc"}`,
		"{\"name\":\"X\"}\n{\"position\":1,\"path\":null}",
		"{\"name\":\"X\"}\n{\"position\":1,\"path\":\"a\"}\n{\"position\":1,\"path\":\"b\"}",
		"{\"done\":true}\n{}",
	} {
		if _, err := decode([]byte(input)); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestActiveJournalAndMissingInterpreter(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "exportLibrary.db")
	if err := os.WriteFile(filename, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TUNA_PYTHON", filename+"-absent-python")
	if _, err := Read(context.Background(), filepath.Dir(filename), filename); !errors.Is(err, errReader) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename+"-wal", []byte("active"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), filepath.Dir(filename), filename); err == nil || !strings.Contains(err.Error(), "journal") {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Read(ctx, filepath.Dir(filename), filename); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSnapshotRejectsChangedSource(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "exportLibrary.db")
	if err := os.WriteFile(filename, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := checkSource(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	changed := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(filename, changed, changed); err != nil {
		t.Fatal(err)
	}
	if err := readStable(context.Background(), filename, info, io.Discard); !errors.Is(err, errChanged) {
		t.Fatal(err)
	}
}

func TestOutputLimitCancelsReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &boundedOutput{limit: 8, cancel: cancel}
	if _, err := io.Copy(out, strings.NewReader("too large")); err == nil || !out.exceeded || !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal(err, out.exceeded, ctx.Err())
	}
	if out.buf.Len() > out.limit {
		t.Fatal("oversized result was retained")
	}
}

func TestPrivateSnapshotUsesSelectedUSBRoot(t *testing.T) {
	drive := t.TempDir()
	// The caller may have resolved the standard rekordbox path through a
	// symlink on the stick. Its parent directory no longer identifies the USB.
	dbdir := filepath.Join(drive, "alternate", "database")
	if err := os.MkdirAll(dbdir, 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(dbdir, "exportLibrary.db")
	if err := os.WriteFile(filename, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	tempOnDrive := filepath.Join(drive, "scratch")
	if err := os.Mkdir(tempOnDrive, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, tempOnDrive)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TUNA_PYTHON", executable)
	if _, err := Read(context.Background(), drive, filename); err == nil || !strings.Contains(err.Error(), "temporary directory points inside the USB") {
		t.Fatalf("did not reject the on-device temp directory: %v", err)
	}
	files, err := os.ReadDir(tempOnDrive)
	if err != nil || len(files) != 0 {
		t.Fatal("wrote inside USB temp directory", files, err)
	}
}

// TUNA_TEST_ONELIBRARY accepts the upstream v1vendi/onelibrary sample database
// at commit 02c6f80c802a635c394e733058502ecf28cf9fc9. Its audio is unnecessary.
func TestPublishedRekordboxExport(t *testing.T) {
	filename := os.Getenv("TUNA_TEST_ONELIBRARY")
	if filename == "" {
		t.Skip("set TUNA_TEST_ONELIBRARY to the published Sample Library export")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != "95b4abed6011dfadb8a534d6e98b88a5d444208ea2798da090cbcf1b410a9dff" {
		t.Fatal("unexpected fixture")
	}
	dbdir := filepath.Join(t.TempDir(), "PIONEER", "rekordbox")
	if err := os.MkdirAll(dbdir, 0700); err != nil {
		t.Fatal(err)
	}
	deviceDB := filepath.Join(dbdir, "exportLibrary.db")
	if err := os.WriteFile(deviceDB, data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Read(context.Background(), filepath.Dir(filepath.Dir(dbdir)), deviceDB)
	if err != nil {
		t.Fatal(err)
	}
	want := []library.Playlist{{Name: "Sample Library", Entries: []library.Entry{
		{Path: "/Contents/Kevin MacLeod/Incompetech/electrodoodle.mp3", Title: "Electrodoodle"},
		{Path: "/Contents/Kevin MacLeod/Electronic Light/cipher.mp3", Title: "Cipher"},
		{Path: "/Contents/Kevin MacLeod/Funkorama/coldfunk.mp3", Title: "Cold Funk"},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	siblings, err := os.ReadDir(dbdir)
	if err != nil || len(siblings) != 1 {
		t.Fatalf("file set changed: %v %v", siblings, err)
	}
	after, err := os.ReadFile(deviceDB)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatal("source database changed", err)
	}
}
