// Package library describes the ordered records a DJ export can supply.
package library

// A Playlist can contain multiple entries that point to the same track.
type Playlist struct {
	Name    string  `json:"name"`
	Entries []Entry `json:"entries"`
}

// Path is the track location recorded in the export, before mount resolution.
type Entry struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}
