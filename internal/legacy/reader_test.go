package legacy

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/villagealchemist/tune-fishin/internal/library"
)

const testPageSize = 512

func fixtureString(s string) []byte {
	return append([]byte{byte(2*(len(s)+1) + 1)}, []byte(s)...)
}

func fixtureTrack(id uint32, title string) []byte {
	row := make([]byte, 136)
	binary.LittleEndian.PutUint16(row, 0x24)
	binary.LittleEndian.PutUint32(row[72:], id)
	binary.LittleEndian.PutUint16(row[128:], uint16(len(row)))
	row = append(row, fixtureString(title)...)
	binary.LittleEndian.PutUint16(row[134:], uint16(len(row)))
	return append(row, fixtureString("/Contents/"+title+".mp3")...)
}

func fixturePage(index, kind uint32, rows [][]byte) []byte {
	page := make([]byte, testPageSize)
	order := binary.LittleEndian
	order.PutUint32(page[4:], index)
	order.PutUint32(page[8:], kind)
	var mask uint16
	cursor, live := 40, 0
	for slot, row := range rows {
		if row == nil {
			continue // Deleted slot
		}
		mask |= 1 << slot
		order.PutUint16(page[testPageSize-6-slot*2:], uint16(cursor-40))
		cursor += copy(page[cursor:], row)
		live++
	}
	order.PutUint32(page[24:], uint32(len(rows)|live<<13))
	page[27] = 0x24
	order.PutUint16(page[testPageSize-4:], mask)
	return page
}

func fixture() []byte {
	order := binary.LittleEndian
	header := make([]byte, testPageSize)
	order.PutUint32(header[4:], testPageSize)
	order.PutUint32(header[8:], 3)
	for i, kind := range []uint32{0, 7, 8} {
		pointer := header[28+i*16:]
		order.PutUint32(pointer, kind)
		order.PutUint32(pointer[8:], uint32(i+1))
		order.PutUint32(pointer[12:], uint32(i+1))
	}
	playlist := make([]byte, 20)
	order.PutUint32(playlist[12:], 7)
	playlist = append(playlist, fixtureString("Friday")...)
	var entries [][]byte
	for _, p := range [][2]uint32{{2, 22}, {3, 11}, {1, 11}} {
		row := make([]byte, 12)
		order.PutUint32(row, p[0])
		order.PutUint32(row[4:], p[1])
		order.PutUint32(row[8:], 7)
		entries = append(entries, row)
	}
	data := append(header, fixturePage(1, 0, [][]byte{fixtureTrack(11, "Intro"), nil, fixtureTrack(22, "Break")})...)
	data = append(data, fixturePage(2, 7, [][]byte{playlist})...)
	return append(data, fixturePage(3, 8, entries)...)
}

func fixturePath(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "export.pdb")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadPreservesOrderAndRepeats(t *testing.T) {
	got, err := Read(context.Background(), fixturePath(t, fixture()))
	if err != nil {
		t.Fatal(err)
	}
	intro := library.Entry{Path: "/Contents/Intro.mp3", Title: "Intro"}
	middle := library.Entry{Path: "/Contents/Break.mp3", Title: "Break"}
	want := []library.Playlist{{Name: "Friday", Entries: []library.Entry{intro, middle, intro}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadRejectsInconsistentLibrary(t *testing.T) {
	order := binary.LittleEndian
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
		want   string
	}{
		{"truncated page", func(b []byte) []byte { return b[:len(b)-1] }, "header"},
		{"missing track", func(b []byte) []byte { order.PutUint32(b[3*testPageSize+44:], 999); return b }, "missing track ID 999"},
		{"duplicate position", func(b []byte) []byte { order.PutUint32(b[3*testPageSize+40:], 3); return b }, "repeats position"},
		{"page cycle", func(b []byte) []byte {
			order.PutUint32(b[40:], 3) // Last track page is no longer the first.
			order.PutUint32(b[testPageSize+12:], 1)
			return b
		}, "broken page chain"},
		{"invalid row start", func(b []byte) []byte { order.PutUint16(b[testPageSize*2-6:], 65535); return b }, "row bounds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Read(context.Background(), fixturePath(t, tc.mutate(fixture()))); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestDeviceString(t *testing.T) {
	tests := []struct {
		b     []byte
		want  string
		valid bool
	}{
		{[]byte{3}, "", true},
		{[]byte{0x40, 7, 0, 0, 'a', 'b', 'c'}, "abc", true},
		{[]byte{0x90, 8, 0, 0, 0x3c, 0xd8, 0xa7, 0xdf}, "🎧", true},
		{nil, "", false},
		{[]byte{1}, "", false},
		{[]byte{7, 'a'}, "", false},
		{[]byte{0x40, 2}, "", false},
		{[]byte{0x90, 5, 0, 0, 0}, "", false},
		{[]byte{0x90, 6, 0, 0, 0, 0xd8}, "", false},
	}
	for _, tc := range tests {
		got, err := deviceString(tc.b, 0)
		if (err == nil) != tc.valid || got != tc.want {
			t.Errorf("deviceString(%x) = %q, %v", tc.b, got, err)
		}
	}
}
