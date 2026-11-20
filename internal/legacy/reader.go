// Package legacy reads playlists from a rekordbox Device Library export.pdb.
// Format reference: https://djl-analysis.deepsymmetry.org/rekordbox-export-analysis/exports.html
package legacy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"unicode/utf16"

	"github.com/villagealchemist/tune-fishin/internal/library"
)

const (
	tracksTable   = 0
	playlistTable = 7
	entriesTable  = 8
	pageHeader    = 40
	trackOffsets  = 94
	trackHeader   = trackOffsets + 21*2
)

var le = binary.LittleEndian

type table struct {
	kind, first, last uint32
}

type database struct {
	data      io.ReaderAt
	pageSize  int
	pageCount int64
	tables    map[uint32]table
}

func readHeader(data io.ReaderAt, size int64) (*database, error) {
	var header [28]byte
	if _, err := data.ReadAt(header[:], 0); err != nil {
		return nil, fmt.Errorf("read Device Library header: %w", err)
	}
	pageSize := int64(le.Uint32(header[4:8]))
	count := int64(le.Uint32(header[8:12]))
	if le.Uint32(header[:4]) != 0 || pageSize < 512 || pageSize > 65536 ||
		size < pageSize || size%pageSize != 0 || 28+count*16 > pageSize {
		return nil, errors.New("invalid Device Library header")
	}
	pointers := make([]byte, count*16)
	if _, err := data.ReadAt(pointers, 28); err != nil {
		return nil, fmt.Errorf("read Device Library table list: %w", err)
	}
	db := &database{data: data, pageSize: int(pageSize), pageCount: size / pageSize, tables: make(map[uint32]table)}
	for len(pointers) >= 16 {
		p := pointers[:16]
		pointers = pointers[16:]
		kind := le.Uint32(p)
		switch kind {
		case tracksTable, playlistTable, entriesTable:
			if _, found := db.tables[kind]; found {
				return nil, fmt.Errorf("duplicate Device Library table %d", kind)
			}
			db.tables[kind] = table{kind, le.Uint32(p[8:12]), le.Uint32(p[12:16])}
		}
	}
	if len(db.tables) != 3 {
		return nil, errors.New("Device Library is missing a track or playlist table")
	}
	return db, nil
}

func (db *database) readTable(ctx context.Context, kind uint32, visit func([]byte) error) error {
	t := db.tables[kind]
	seen := make(map[uint32]bool)
	page := make([]byte, db.pageSize)
	for index := t.first; ; index = le.Uint32(page[12:16]) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if index == 0 || int64(index) >= db.pageCount || seen[index] {
			return fmt.Errorf("broken page chain at page %d in Device Library table %d", index, kind)
		}
		seen[index] = true
		if _, err := db.data.ReadAt(page, int64(index)*int64(db.pageSize)); err != nil {
			return fmt.Errorf("read Device Library page %d: %w", index, err)
		}
		if le.Uint32(page[4:8]) != index || le.Uint32(page[8:12]) != kind {
			return fmt.Errorf("mismatched Device Library page pointer at page %d", index)
		}
		if page[27]&0x40 == 0 { // Index pages contain no playlist or track rows.
			if err := visitRows(page, visit); err != nil {
				return fmt.Errorf("Device Library page %d: %w", index, err)
			}
		}
		if index == t.last {
			return nil
		}
	}
}

func visitRows(page []byte, visit func([]byte) error) error {
	counts := int(page[24]) | int(page[25])<<8 | int(page[26])<<16
	slots, live := counts&0x1fff, counts>>13
	groups := (slots + 15) / 16
	heapEnd := len(page) - slots*2 - groups*4
	if heapEnd < pageHeader {
		return errors.New("row index overlaps the page header")
	}
	// The row index grows backwards. Deleted slots can contain stale offsets.
	offsets := make([]int, 0, live+1)
	for i := 0; i < slots; i++ {
		end := len(page) - (i/16)*36
		slot := i % 16
		if le.Uint16(page[end-4:])&(1<<slot) != 0 {
			offsets = append(offsets, pageHeader+int(le.Uint16(page[end-6-slot*2:])))
		}
	}
	if len(offsets) != live {
		return errors.New("row count disagrees with the slot index")
	}
	sort.Ints(offsets)
	offsets = append(offsets, heapEnd)
	for i := 0; i < live; i++ {
		start, end := offsets[i], offsets[i+1]
		if start >= end || end > heapEnd {
			return errors.New("overlapping or invalid row bounds")
		}
		if err := visit(page[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func deviceString(row []byte, offset int) (string, error) {
	if offset < 0 || offset >= len(row) {
		return "", errors.New("string points outside the row")
	}
	s := row[offset:]
	header, size := 1, int(s[0]>>1)
	kind := s[0]
	if kind&1 == 0 {
		if len(s) < 4 || (kind != 0x40 && kind != 0x90) {
			return "", fmt.Errorf("unsupported Device Library string format %#x", kind)
		}
		header, size = 4, int(le.Uint16(s[1:3]))
	}
	if size < header || size > len(s) {
		return "", errors.New("invalid string length")
	}
	payload := s[header:size]
	if kind != 0x90 {
		for _, b := range payload {
			if b > 127 {
				return "", errors.New("invalid ASCII string")
			}
		}
		return string(payload), nil
	}
	if len(payload)%2 != 0 {
		return "", errors.New("invalid UTF-16 string length")
	}
	var chars []rune
	for len(payload) > 0 {
		ch := rune(le.Uint16(payload))
		payload = payload[2:]
		if utf16.IsSurrogate(ch) {
			if len(payload) < 2 {
				return "", errors.New("incomplete UTF-16 surrogate pair")
			}
			ch = utf16.DecodeRune(ch, rune(le.Uint16(payload)))
			if ch == '\uFFFD' {
				return "", errors.New("invalid UTF-16 surrogate pair")
			}
			payload = payload[2:]
		}
		chars = append(chars, ch)
	}
	return string(chars), nil
}

type positionedEntry struct {
	position uint32
	track    library.Entry
}

// Read returns playlists in tree-row order and their tracks in set order.
// Playlists sharing a name and tracks repeated in one playlist stay distinct.
func Read(ctx context.Context, path string) ([]library.Playlist, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	db, err := readHeader(file, info.Size())
	if err != nil {
		return nil, err
	}
	tracks := make(map[uint32]library.Entry)
	err = db.readTable(ctx, tracksTable, func(row []byte) error {
		if len(row) < trackHeader || le.Uint16(row) != 0x24 {
			return errors.New("unsupported track record layout")
		}
		id := le.Uint32(row[72:76])
		if _, exists := tracks[id]; exists {
			return fmt.Errorf("duplicate track ID %d", id)
		}
		titleAt := int(le.Uint16(row[trackOffsets+17*2:]))
		pathAt := int(le.Uint16(row[trackOffsets+20*2:]))
		if titleAt < trackHeader || pathAt < trackHeader {
			return errors.New("invalid string offset in track record")
		}
		title, err := deviceString(row, titleAt)
		if err != nil {
			return err
		}
		path, err := deviceString(row, pathAt)
		if err != nil {
			return err
		}
		tracks[id] = library.Entry{Path: path, Title: title}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var result []library.Playlist
	indices := make(map[uint32]int)
	err = db.readTable(ctx, playlistTable, func(row []byte) error {
		if len(row) < 21 {
			return errors.New("incomplete playlist row")
		}
		if le.Uint32(row[16:20]) != 0 {
			return nil // Folder
		}
		id := le.Uint32(row[12:16])
		if _, exists := indices[id]; exists {
			return fmt.Errorf("duplicate playlist ID %d", id)
		}
		name, err := deviceString(row, 20)
		if err != nil {
			return err
		}
		indices[id] = len(result)
		result = append(result, library.Playlist{Name: name})
		return nil
	})
	if err != nil {
		return nil, err
	}
	byPlaylist := make([][]positionedEntry, len(result))
	err = db.readTable(ctx, entriesTable, func(row []byte) error {
		if len(row) < 12 {
			return errors.New("incomplete playlist entry")
		}
		pos, trackID, playlistID := le.Uint32(row), le.Uint32(row[4:]), le.Uint32(row[8:])
		idx, ok := indices[playlistID]
		if !ok {
			return fmt.Errorf("missing playlist ID %d", playlistID)
		}
		track, ok := tracks[trackID]
		if !ok {
			return fmt.Errorf("playlist %q refers to missing track ID %d", result[idx].Name, trackID)
		}
		byPlaylist[idx] = append(byPlaylist[idx], positionedEntry{pos, track})
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i, entries := range byPlaylist {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].position < entries[j].position })
		for j, item := range entries {
			if j > 0 && entries[j-1].position == item.position {
				return nil, fmt.Errorf("playlist %q repeats position %d", result[i].Name, item.position)
			}
			result[i].Entries = append(result[i].Entries, item.track)
		}
	}
	return result, nil
}
