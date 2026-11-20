// Package ui presents the USB check through local operating-system dialogs.
package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/villagealchemist/tune-fishin/internal/preflight"
)

const title = "Tune Fishin · Check my gig USB"

var errCanceled = errors.New("dialog canceled")

// Run collects a set, device and deck, then displays the read-only USB report.
func Run(ctx context.Context) error {
	dialog, err := newDialogs()
	if err != nil {
		return err
	}
	err = run(ctx, dialog, preflight.Run)
	if errors.Is(err, errCanceled) {
		return nil
	}
	return err
}

type request struct {
	Kind    string   `json:"kind"`
	Prompt  string   `json:"prompt"`
	Default string   `json:"default"`
	Choices []string `json:"choices,omitempty"`
}

type askFunc func(context.Context, request) (string, error)
type checkFunc func(context.Context, preflight.Config) (preflight.Report, error)

func run(ctx context.Context, ask askFunc, check checkFunc) error {
	decks := make([]string, 0, len(preflight.Decks))
	for deck := range preflight.Decks {
		decks = append(decks, deck)
	}
	sort.Strings(decks)
	var cfg preflight.Config
	var err error
	cfg.Deck, err = ask(ctx, request{Kind: "choice", Prompt: "Which deck will you play on?", Choices: decks})
	if err != nil {
		return err
	}
	if _, ok := preflight.Decks[cfg.Deck]; !ok {
		return fmt.Errorf("unrecognized deck %q", cfg.Deck)
	}
	cfg.Source, err = ask(ctx, request{Kind: "file", Prompt: "Choose your intended set (.m3u8 or .m3u)"})
	if err != nil {
		return err
	}
	cfg.USB, err = ask(ctx, request{Kind: "folder", Prompt: "Choose the top-level folder of the mounted gig USB"})
	if err != nil {
		return err
	}
	base := filepath.Base(cfg.Source)
	cfg.Playlist, err = ask(ctx, request{
		Kind: "entry", Prompt: "Which playlist on the USB should match your set?",
		Default: strings.TrimSuffix(base, filepath.Ext(base)),
	})
	if err != nil {
		return err
	}
	const direct = "Read the library on the USB"
	const snapshot = "Use a device playlist export if the library cannot be read"
	mode, err := ask(ctx, request{
		Kind: "choice", Prompt: "How should Tune Fishin check the device playlist?",
		Choices: []string{direct, snapshot}, Default: direct,
	})
	if err != nil {
		return err
	}
	if mode == snapshot {
		cfg.Snapshot, err = ask(ctx, request{Kind: "file", Prompt: fmt.Sprintf(
			"Choose a fresh playlist export from rekordbox → Devices → your USB → %s → Playlists",
			preflight.Decks[cfg.Deck],
		)})
		if err != nil {
			return err
		}
	} else if mode != direct {
		return fmt.Errorf("unrecognized playlist source %q", mode)
	}
	report, err := checkWithProgress(ctx, ask, check, cfg)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, errCanceled) {
		return err
	}
	if err != nil {
		_, displayErr := ask(ctx, request{Kind: "report", Prompt: "Check incomplete", Default: err.Error()})
		if displayErr != nil {
			return displayErr
		}
		return err
	}
	_, err = ask(ctx, request{Kind: "report", Prompt: report.Status, Default: report.Text})
	return err
}

func checkWithProgress(ctx context.Context, ask askFunc, check checkFunc, cfg preflight.Config) (preflight.Report, error) {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	type outcome struct {
		report preflight.Report
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		report, err := check(ctx, cfg)
		finished <- outcome{report, err}
	}()

	// The modal is a separate helper process. Closing it cancels the same check
	// that the CLI uses, and completing the check dismisses the modal.
	closed := make(chan error, 1)
	go func() {
		_, err := ask(ctx, request{Kind: "busy", Prompt: "Reading your USB and comparing the audio. Keep the drive connected."})
		closed <- err
	}()
	select {
	case result := <-finished:
		stop()
		<-closed
		return result.report, result.err
	case err := <-closed:
		stop()
		<-finished
		if err != nil {
			return preflight.Report{}, err
		}
		return preflight.Report{}, errCanceled
	}
}
