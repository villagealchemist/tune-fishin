# 🌊 TUNA eFISHIN't
*Because everything else is TUNA eFISHIN't (too inefficient)*

One of the most difficult alchemical skills to master is the transmutation of failure into flow, for the master of all knows they are the master of none.

TUNA eFISHIN't is that transmutation made tangible — a spark born from a slip of the tongue, where "too inefficient" became "tuna fishin'." What began as mishearing became meaning: proof that even static can sing.

> When they tryna make you into Ahab, I say nooo, nooo, nooo. No maming. No hunt. Just endless access to hot single WhiteWhales in your area or anywhere audio can be recorded, transmuted, and set free. 🐋⚗️

---

## 🎧 Check my gig USB

Check the stick you plan to take to the venue. Tune Fishin reads the exported playlist for the deck you select, compares its order with your intended set, and checks the audio files on the stick against your originals. It reads the filesystem format where the OS exposes it. Everything runs on your computer.

1. In rekordbox, right-click your intended playlist under **Playlists**. Choose **Export a playlist to a file** and save it as **M3U8** on your computer. Keep the original music drive connected.
2. Export the set to your gig USB in rekordbox as usual. Wait for the export to finish, eject the USB, and reconnect it.
3. With Python 3.10+ and Tkinter installed, open **Check Gig USB.command** on macOS, or run `python3 gig_usb/tuna.py` from the project folder. Pick the venue deck, your intended M3U8, and the **root** of the USB. Click **Check USB**. If you renamed the M3U8, enter the actual USB playlist name under the extra options.

**What the result means**

- **MATCH** means the selected deck library contains the same audio files in the same order (including repeats), and the filesystem check passed. This checks playlist data and file bytes. Playback, codec compatibility, partition layout, cues, and beatgrids require separate checks.
- **DIFFERENCES** identifies a missing playlist, omitted or unexpected tracks, order changes, or a known filesystem incompatibility. For omitted tracks the checker looks for identical audio elsewhere on the stick. A different edit, a transcode, or new embedded tags change the file fingerprint.
- **INCOMPLETE** identifies anything the checker could not verify, such as an unavailable original, an unreadable file, or an unknown filesystem. A backup folder can be used to inspect playlist content. Select the mounted volume to check its format.

**Deck support**

| Deck | USB library checked |
| --- | --- |
| CDJ-2000NXS2, XDJ-XZ, XDJ-RX3, CDJ-3000 | Device Library |
| CDJ-3000X, XDJ-AZ, OPUS-QUAD, OMNIS-DUO | OneLibrary |
| CDJ-3000 specifically running withdrawn firmware 3.30 | Choose the separate 3.30 option to check OneLibrary |

Device Library checks use Python's standard library. Direct OneLibrary checks also need Python 3.11+ and the optional reader in the Python environment used to launch the app.

```bash
python3 -m pip install 'pyrekordbox==0.5.0' sqlcipher3-wheels
```

If the reader is unavailable, open **Devices → your USB → OneLibrary → Playlists** in rekordbox and export the **device** playlist as a fresh M3U8. Select it under the checker's extra options. The snapshot must reference audio on the mounted stick. Its results are labeled separately because it captures the playlist at the time of export.

**Terminal usage**

```bash
python3 gig_usb/tuna.py check --set ~/Music/Friday.m3u8 --usb "/Volumes/GIG USB" --deck CDJ-2000NXS2
```

Use `--playlist "Friday closer"` if the USB playlist has a different name. Use `--snapshot path/to/device.m3u8` for the snapshot fallback. Exit codes are `0` for a match, `1` for differences, and `2` for an incomplete check. Run the test suite with `python3 -m unittest discover -s gig_usb`.

[rekordbox USB compatibility](https://rekordbox.com/en/support/usb-export/) · [AlphaTheta firmware notice](https://alphatheta.com/en/information/important-notice-for-customers-using-usb-devices-with-our-dj-equipment/)

---

## ⚗️ What This Actually Does

**TUNA eFISHIN't extracts clean audio from videos.** That's it. No sketchy sites, no quality loss, no hunting through ad mazes.

**Current reality:** CLI tool + browser extension  
**Coming soon:** Desktop apps + mobile magic

**`video file` → `clean .mp3` → `your DJ library`**

---

## 🔱 The Fleet (v1.0 → Future)

### 🖥️ CLI (Available Now)
```bash
tuna extract video.mp4               # Single track extraction
tuna batch /folder/                  # Batch processing  
tuna range video.mp4 1:30 4:45      # Precise time ranges
```

### 🌐 Browser Extension (Available Now)
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

## 🧪 Installation

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