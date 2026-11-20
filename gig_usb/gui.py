"""Desktop window for the read-only Tune Fishin USB check."""

from pathlib import Path
from queue import Empty, Queue
from threading import Thread
import tkinter as tk
from tkinter import filedialog, ttk
from tkinter.scrolledtext import ScrolledText

from preflight import DECKS, check


PLAYLIST_FILES = [("Playlists", ("*.m3u8", "*.m3u")), ("All files", "*")]
STATUS_LABELS = {
    "MATCH": "Checks passed",
    "DIFFERENCES": "Differences found",
    "INCOMPLETE": "Check incomplete",
}


class PreflightWindow:
    def __init__(self, root: tk.Tk):
        self.root = root
        root.title("Tune Fishin · Check my gig USB")
        root.minsize(580, 560)
        root.columnconfigure(0, weight=1)
        root.rowconfigure(0, weight=1)

        self.results = Queue()
        self.controls = []
        self.deck = tk.StringVar()
        self.source = tk.StringVar()
        self.usb = tk.StringVar()
        self.playlist = tk.StringVar()
        self.snapshot = tk.StringVar()
        self.snapshot_hint = tk.StringVar()
        self.status = tk.StringVar()
        self.busy = False

        panel = ttk.Frame(root, padding=18)
        panel.grid(sticky="nsew")
        panel.columnconfigure(1, weight=1)
        ttk.Label(panel, text="Check my gig USB", font="TkHeadingFont").grid(
            row=0, column=0, columnspan=3, sticky="w", pady=(0, 4)
        )
        ttk.Label(
            panel,
            text="Compare your intended set with the playlist exported for the venue’s deck.",
            wraplength=540,
        ).grid(row=1, column=0, columnspan=3, sticky="w", pady=(0, 12))

        ttk.Label(panel, text="Venue deck").grid(row=2, column=0, sticky="w")
        deck_picker = ttk.Combobox(
            panel, textvariable=self.deck, values=sorted(DECKS), state="readonly"
        )
        deck_picker.grid(row=2, column=1, columnspan=2, sticky="ew", pady=4)
        self.controls.append((deck_picker, "readonly"))
        self.path_field(panel, 3, "Intended set (.m3u8 / .m3u)", self.source)
        self.path_field(panel, 4, "Mounted USB folder", self.usb, folder=True)

        self.show_options = tk.BooleanVar()
        self.options = ttk.Frame(panel)
        self.options.columnconfigure(1, weight=1)
        extra = ttk.Checkbutton(
            panel,
            text="Use a different USB playlist name or a device playlist export",
            variable=self.show_options,
            command=self.toggle_options,
        )
        extra.grid(row=5, column=0, columnspan=3, sticky="w", pady=4)
        self.controls.append((extra, "normal"))
        self.options.grid(row=6, column=0, columnspan=3, sticky="ew")
        ttk.Label(self.options, text="USB playlist name").grid(row=0, column=0, sticky="w")
        name = ttk.Entry(self.options, textvariable=self.playlist)
        name.grid(row=0, column=1, columnspan=2, sticky="ew", padx=(10, 0), pady=4)
        self.controls.append((name, "normal"))
        ttk.Label(
            self.options, text="Leave blank to use the intended set’s filename without its extension.",
            wraplength=540,
        ).grid(row=1, column=0, columnspan=3, sticky="w")
        self.path_field(self.options, 2, "Device playlist export", self.snapshot)
        ttk.Label(self.options, textvariable=self.snapshot_hint, wraplength=540).grid(
            row=3, column=0, columnspan=3, sticky="w", pady=(0, 4)
        )
        self.options.grid_remove()
        self.deck.trace_add("write", self.update_hint)
        self.update_hint()

        actions = ttk.Frame(panel)
        actions.grid(row=7, column=0, columnspan=3, sticky="ew", pady=8)
        self.run = ttk.Button(actions, text="Check USB", command=self.start)
        self.run.pack(side="left")
        self.progress = ttk.Progressbar(actions, mode="indeterminate", length=100)
        ttk.Label(actions, textvariable=self.status).pack(side="right")

        self.report = ScrolledText(
            panel, height=12, width=68, wrap="word", padx=9, pady=8, state="disabled",
            font="TkTextFont",
        )
        self.report.grid(row=8, column=0, columnspan=3, sticky="nsew")
        panel.rowconfigure(8, weight=1)
        deck_picker.focus_set()

    def path_field(self, parent, row, label, variable, folder=False):
        ttk.Label(parent, text=label).grid(row=row, column=0, sticky="w")
        entry = ttk.Entry(parent, textvariable=variable)
        entry.grid(row=row, column=1, sticky="ew", padx=(10, 6), pady=4)

        def choose():
            if folder:
                selected = filedialog.askdirectory(parent=self.root, title=label, mustexist=True)
            else:
                selected = filedialog.askopenfilename(
                    parent=self.root, title=label, filetypes=PLAYLIST_FILES
                )
            if selected:
                variable.set(selected)

        picker = ttk.Button(parent, text="Browse…", command=choose)
        picker.grid(row=row, column=2, sticky="e")
        self.controls.extend([(entry, "normal"), (picker, "normal")])

    def update_hint(self, *_):
        library = DECKS.get(self.deck.get(), "the deck’s library")
        self.snapshot_hint.set(
            f"For a snapshot check, export the playlist in rekordbox under Devices → "
            f"your USB → {library} → Playlists. Choose that fresh export here."
        )

    def toggle_options(self):
        if self.show_options.get():
            self.options.grid()
        else:
            self.options.grid_remove()

    def show_result(self, status, content):
        self.status.set(status)
        self.report.config(state="normal")
        self.report.delete("1.0", "end")
        self.report.insert("1.0", content)
        self.report.config(state="disabled")

    def start(self):
        if self.busy:
            return
        deck = self.deck.get()
        if not (deck in DECKS and self.source.get().strip() and self.usb.get().strip()):
            self.show_result("Select inputs", "Choose a venue deck, intended set, and mounted USB folder.")
            return

        source = Path(self.source.get()).expanduser()
        usb = Path(self.usb.get()).expanduser()
        name = None
        snapshot = None
        if self.show_options.get():
            name = self.playlist.get().strip() or None
            if self.snapshot.get().strip():
                snapshot = Path(self.snapshot.get()).expanduser()

        self.busy = True
        for control, _ in self.controls:
            control.config(state="disabled")
        self.run.config(state="disabled")
        self.show_result("Checking USB…", "")
        self.progress.pack(side="left", padx=10)
        self.progress.start()

        def worker():
            try:
                report = check(usb, source, deck, playlist=name, snapshot=snapshot)
                result = STATUS_LABELS[report.status], report.render()
            except (OSError, ValueError) as error:
                result = "Check incomplete", str(error)
            except Exception as error:
                result = "Check incomplete", f"Unexpected error ({type(error).__name__}): {error}"
            self.results.put(result)

        Thread(target=worker, daemon=True).start()
        self.root.after(100, self.poll)

    def poll(self):
        try:
            status, content = self.results.get_nowait()
        except Empty:
            self.root.after(100, self.poll)
            return
        self.progress.stop()
        self.progress.pack_forget()
        for control, state in self.controls:
            control.config(state=state)
        self.run.config(state="normal")
        self.busy = False
        self.show_result(status, content)


def launch():
    root = tk.Tk()
    PreflightWindow(root)
    root.mainloop()


if __name__ == "__main__":
    launch()
