# 🌊 TUNA eFISHIN't
*Because everything else is TUNA eFISHIN't (too inefficient)*

One of the most difficult alchemical skills to master is the transmutation of failure into flow, for the master of all knows they are the master of none.

TUNA eFISHIN't is that transmutation made tangible — a spark born from a slip of the tongue, where "too inefficient" became "tuna fishin'." What began as mishearing became meaning: proof that even static can sing.

> When they tryna make you into Ahab, I say nooo, nooo, nooo. No maming. No hunt. Just endless access to hot single WhiteWhales in your area or anywhere audio can be recorded, transmuted, and set free. 🐋⚗️

---

## 🎧 Check my gig USB

Tune Fishin reads a set back from the stick you will take to the venue. It checks the playlist in the library used by your selected deck, compares the actual audio files byte for byte with your originals, and checks the mounted volume's filesystem. It runs locally and reads the USB without writing to it.

### Run the checker

From this branch, build once with Go 1.23 or later. The binary runs on its own for the standard Device Library workflow.

```bash
go build -o tuna ./cmd/tuna
./tuna
```

On macOS you can also double-click **Check Gig USB.command** beside the binary. On Windows build with `go build -o tuna.exe ./cmd/tuna` and open `tuna.exe`. Linux uses Zenity for graphical dialogs. The terminal command works on all three platforms.

1. In rekordbox, right-click the intended set under **Playlists**, choose **Export a playlist to a file**, and save a local **M3U8**. Keep the original music accessible.
2. Export the set to the USB in rekordbox. After export completes, eject the stick and reconnect it.
3. Open Tune Fishin, pick the venue deck, the intended M3U8, and the root of the USB. Confirm the playlist name as it appears on the device.

```bash
./tuna check --set ~/Music/Friday.m3u8 --usb "/Volumes/GIG USB" --deck CDJ-2000NXS2
```

If the set has a different name on the device, add `--playlist "Friday closer"`. Use `tuna check --help` for the full list of models.

| Result | What the checker established |
| --- | --- |
| MATCH · exit 0 | The selected deck library lists the same files in the same order, including repeats, the audio bytes match, and the USB filesystem check passed. |
| DIFFERENCES · exit 1 | The checker found a missing library or playlist, a track or order difference, or a known filesystem incompatibility. If a track is missing from the playlist, it also looks on the USB for a byte-identical copy. |
| INCOMPLETE · exit 2 | Evidence was unavailable, such as an unreadable original, an unknown filesystem, or a match established only from a playlist snapshot. Backup folders can be used to inspect set contents. |

A transcode or tag edit changes a file's SHA-256. A match covers file contents, set order, and filesystem format. Playback on the deck, codec support, partition layout, cues, and beatgrids need separate checks.

### Deck libraries

| Model | Library read from the USB |
| --- | --- |
| CDJ-2000NXS2, XDJ-XZ, XDJ-RX3, CDJ-3000 | Device Library (`export.pdb`) |
| CDJ-3000X, XDJ-AZ, OPUS-QUAD, OMNIS-DUO | OneLibrary (`exportLibrary.db`) |
| CDJ-3000 running withdrawn firmware 3.30 | Select **CDJ-3000 (firmware 3.30)** to check OneLibrary |

The Device Library reader is native Go. Direct inspection of an encrypted OneLibrary export is the one workflow that uses Python. The optional adapter runs against a private copy of the database. To enable it, create a Python 3.11+ virtual environment on your computer, install the reader, and point `TUNA_PYTHON` at that interpreter. For example, on macOS or Linux:

```bash
python3 -m venv ~/.venvs/tuna-onelibrary
~/.venvs/tuna-onelibrary/bin/python -m pip install 'pyrekordbox==0.5.0' sqlcipher3-wheels
export TUNA_PYTHON="$HOME/.venvs/tuna-onelibrary/bin/python"
./tuna
```

Alternatively, export a fresh device M3U8 from **Devices → your USB → OneLibrary → Playlists** in rekordbox and supply it through the dialogs or `--snapshot path/to/device.m3u8`. The entries must resolve to audio on the selected USB. The checker tries the live library first and identifies snapshot evidence explicitly.

### Follow the code

[DJ software, hardware and the export formats between them](docs/dj-stack.md) maps the vendor stacks and follows a track through the Go processor.

- `cmd/tuna` and `internal/ui` take the same inputs and call `preflight.Run`.
- `internal/legacy` reads little-endian pages from `export.pdb` using Go's `io.ReaderAt`. `internal/onelibrary` runs the optional reader on a temporary copy. Both return ordered `library.Playlist` records.
- `internal/preflight` resolves device paths, streams SHA-256 hashes, and compares track occurrences in order. `internal/volume` asks the host OS about the mounted device.

Run `go test ./...` from the repository root.

[rekordbox USB compatibility](https://rekordbox.com/en/support/usb-export/) · [AlphaTheta firmware notice](https://alphatheta.com/en/information/important-notice-for-customers-using-usb-devices-with-our-dj-equipment/) · [Device Library page format](https://djl-analysis.deepsymmetry.org/rekordbox-export-analysis/exports.html)

---

## ⚗️ The original extraction roadmap

**The original idea for TUNA eFISHIN't was to extract clean audio from videos.** No sketchy sites, no quality loss, no hunting through ad mazes.

The commands, extension, and distribution channels below describe the original plan. The gig USB checker above is implemented on this branch.

**`video file` → `clean .mp3` → `your DJ library`**

---

## 🔱 The Fleet (v1.0 → Future)

### 🖥️ CLI (Planned)
```bash
tuna extract video.mp4               # Single track extraction
tuna batch /folder/                  # Batch processing  
tuna range video.mp4 1:30 4:45      # Precise time ranges
```

### 🌐 Browser Extension (Planned)
Right-click any YouTube/SoundCloud/TikTok video → instant MP3 download

### 💻 Desktop Apps (Soon)
Drag-and-drop GUI for the terminal-averse. Native .exe/.dmg files.

### 📱 Mobile Apps (Eventually)
Screen record from any app → auto-extract audio → sync to your library

**ProTip:** You can already do this manually today:
- **iPhone:** Control Center → Screen Recording (hold to enable microphone)
- **Android:** Quick Settings → Screen Record → Internal Audio
- Then use the CLI to extract audio from your recordings

---

## 🧪 Planned distribution for the extraction tools

**CLI:**
```bash
# Via Homebrew (macOS/Linux)
brew install villagealchemist/tap/tuna-efishint

# Via Go
go install github.com/villagealchemist/tuna-efishint@latest

# Or download binary from releases
```

**Browser Extension:**
Chrome Web Store → "TUNA eFISHIN't" → Add to Chrome

**Requirements:** None (ffmpeg bundled)

---

## 🌙 Mariner's Musings

*[Message in a bottle, ejected from a temporal wormhole near the Bermuda Triangle. Dated October 22, 2025 - one week hence]*

*"The streaming leviathans grow fat on hoarded frequencies, their DRM tentacles choking the free flow of sound. But we alchemists know the ancient truth: transformation, not theft.*

*We do not steal the audio—we transmute it. Wild frequencies, captured through honest means, refined in our digital crucibles until pure .mp3 empathy emerges.*

*Remember this, young transmuter: fishin' for tracks ain't the same as fishin' for compliments. The work is sacred. The method is sound."*

*—Mariner's Log, Final Entry*

---

## ⚖️ Legal Waters

**Use responsibly.** Extract audio you have rights to use. Respect creators, support artists, follow your local laws. TUNA eFISHIN't is for personal backup, DJ prep, and creative workflows—not mass piracy.

*Mariner Alchemist sails international waters - navigate your local laws accordingly.*

---

## 🏴‍☠️ The Crew

**Mariner Alchemist** (@villagealchemist)  
*Transmutation Specialist, Mayo Mystic, Pun Pirate*

*"I've sailed these digital waters since the age of dial-up. Good audio, properly transmuted, becomes empathy."*

---

## 🗝️ License

**MIT License** © 2025 The SS TUNA eFISHIN't

*Take the bait. Use the tools. Transform the inefficient. With great transmutation comes great responsibility.*

---

*Hurricane winds make for the stormiest of seas, but TUNA eFISHIN't cuts through the chaos like a blessed harpoon through digital flesh.*

**The voyage continues.**