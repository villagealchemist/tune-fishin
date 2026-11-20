// Command tuna checks the set prepared on a DJ's computer against a USB export.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/villagealchemist/tune-fishin/internal/preflight"
	"github.com/villagealchemist/tune-fishin/internal/ui"
)

const usage = `Usage: tuna                       open the desktop checker
       tuna check --set SET.m3u8 --usb MOUNT --deck MODEL [--playlist NAME] [--snapshot DEVICE.m3u8]

Exit codes for check: 0 match, 1 differences, 2 incomplete or invalid input.`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		if err := ui.Run(ctx); err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
		return 0
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprintln(out, usage)
		return 0
	case "check":
	default:
		fmt.Fprintf(errOut, "Unknown command %q\n%s\n", args[0], usage)
		return 2
	}

	var cfg preflight.Config
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(errOut)
	flags.StringVar(&cfg.Source, "set", "", "intended local M3U8 or UTF-8 M3U")
	flags.StringVar(&cfg.USB, "usb", "", "mounted USB root")
	flags.StringVar(&cfg.Deck, "deck", "", "venue deck model")
	flags.StringVar(&cfg.Playlist, "playlist", "", "device playlist name (default: intended filename without extension)")
	flags.StringVar(&cfg.Snapshot, "snapshot", "", "fresh device playlist M3U8 to use if the library cannot be read")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), usage)
		flags.PrintDefaults()
		decks := make([]string, 0, len(preflight.Decks))
		for model := range preflight.Decks {
			decks = append(decks, model)
		}
		sort.Strings(decks)
		fmt.Fprintln(flags.Output(), "Supported decks:", strings.Join(decks, ", "))
	}
	if err := flags.Parse(args[1:]); err == flag.ErrHelp {
		return 0
	} else if err != nil {
		return 2
	}
	if flags.NArg() != 0 || cfg.Source == "" || cfg.USB == "" || cfg.Deck == "" {
		flags.Usage()
		return 2
	}
	report, err := preflight.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintln(errOut, "INCOMPLETE |", err)
		return 2
	}
	fmt.Fprintln(out, report.Text)
	switch report.Status {
	case "MATCH":
		return 0
	case "DIFFERENCES":
		return 1
	default:
		return 2
	}
}
