# What runs where in a DJ setup

When Max moves a track from slot 12 to slot 3, three pieces of state can change independently. Sorting the screen by key changes a view. Saving an ordered playlist changes the relationship between a playlist, an ordinal, and a track ID. Exporting it materializes a copy for a particular player. A song can be present on the USB while the player's playlist has no entry for it.

## What the vendors expose

| System | Published implementation details and interoperability surface |
| --- | --- |
| AlphaTheta / rekordbox | The company identifies C++ for its desktop DJ application and embedded DJ software. The CDJ-3000 board build targets ARM64 with Poky. The older USB export uses little-endian database pages and ordered track references. [AlphaTheta submission, §6.19](https://comcom.govt.nz/__data/assets/pdf_file/0027/349515/AlphaTheta-Submission-in-response-to-Statement-of-Issues-9-April-2024.pdf) · [CDJ-3000 build notes](https://files.microcms-assets.io/assets/3b9e29ce734e49babfedb3f8d1e728e3/a73b16e493034c46ae1ccda938f3e6de/94F586AA-D5A5-4C6B-8261-3BC35D9359C8.txt) · [Device Library format](https://djl-analysis.deepsymmetry.org/rekordbox-export-analysis/exports.html) |
| Serato | Its legal notices identify RtMidi's C++ MIDI classes. Its support docs identify the Subcrates directory and version-specific legacy files. The full private application stack is undisclosed. [Software notices](https://serato.com/legal/software-legal-notices) · [Library layout](https://support.serato.com/hc/en-us/articles/204022904-What-is-in-the-Serato-folder) |
| Denon / Engine DJ | Engine Desktop uses Qt 6 for graphics. Denon documents read-only SQLite access and keys for joining tracks and playlists with an external app's data. [Desktop framework](https://inmusicsupport.freshdesk.com/en/support/solutions/articles/69000881479-engine-dj-desktop-legacy-intel-graphics-hardware-compatibility) · [Third-party database guide](https://support.denondj.com/en/support/solutions/articles/69000834165-engine-dj-v3-0-support-for-third-party-database-tools) |
| Native Instruments / Traktor | NI documents the local collection.nml file and ships a local conversion tool that reads it. This establishes a library boundary independently of how the playback engine is implemented. [NI collection workflow](https://support.native-instruments.com/support/solutions/articles/69000879381-traktor-how-to-convert-your-beatsource-tracks-to-beatport) |
| Mixxx | Open-source C++ and Qt with Linux support and JavaScript controller mappings. [Core](https://mixxx.org/get-involved/) · [Platforms](https://mixxx.org/download/) · [Controller mappings](https://github.com/mixxxdj/mixxx/wiki) |
| Apple / Google | At the playback layer, Core Audio exposes C and Objective-C interfaces, and Android exposes AAudio and the Oboe C++ wrapper. The USB checker operates on files and volume information. [Apple](https://developer.apple.com/library/archive/documentation/MusicAudio/Conceptual/CoreAudioOverview/Introduction/Introduction.html) · [Android](https://developer.android.com/ndk/guides/audio/index.html) |

## Follow one track through Go

For a legacy rekordbox export, the reader opens `PIONEER/rekordbox/export.pdb`. The first page describes the page size and locates the tables. On each data page, the reader checks the bitmap to identify live rows and decodes the little-endian fields. A playlist entry gives it a position, track ID and playlist ID. The track record supplies the device path and title. A track can appear at multiple positions.

[Go page reader](../internal/legacy/reader.go) · [Format analysis](https://djl-analysis.deepsymmetry.org/rekordbox-export-analysis/exports.html)

That produces an ordered `library.Playlist`. Go resolves each path under the selected USB root and streams the file through SHA-256. It consumes a separate occurrence for every match. If the playlist omits a track, it searches the drive for the same bytes under another name. That gives a DJ two different answers to the question “where did my closer go?” The audio can be missing from the playlist, from the stick, or from both.

[Playlist model](../internal/library/playlist.go) · [Reconciliation](../internal/preflight/compare.go)

The boundary for a future Engine DJ or Serato connector is that same ordered record. A connector speaks its vendor's file format. The Go processor owns identity, order, and the evidence behind the result. Native audio callbacks, controller input, and hardware playback belong to different parts of the system.
