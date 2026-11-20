package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/villagealchemist/tune-fishin/internal/preflight"
)

func TestUSBWorkflow(t *testing.T) {
	for _, snapshot := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "snapshot fallback"}[snapshot], func(t *testing.T) {
			want := preflight.Config{Deck: "CDJ-3000X", Source: "/music/Set.m3u8", USB: "/mnt/Gig", Playlist: "Friday"}
			if snapshot {
				want.Snapshot = "/exports/USB.m3u8"
			}
			var displayed string
			ask := func(ctx context.Context, r request) (string, error) {
				switch r.Kind {
				case "busy":
					<-ctx.Done()
					return "", ctx.Err()
				case "choice":
					if r.Default == "" {
						return want.Deck, nil
					}
					if snapshot {
						return r.Choices[1], nil
					}
					return r.Default, nil
				case "file":
					if strings.Contains(r.Prompt, "intended") {
						return want.Source, nil
					}
					if !strings.Contains(r.Prompt, "OneLibrary") {
						return "", errors.New("wrong device library in snapshot picker")
					}
					return want.Snapshot, nil
				case "folder":
					return want.USB, nil
				case "entry":
					if r.Default != "Set" {
						return "", errors.New("wrong playlist default")
					}
					return want.Playlist, nil
				case "report":
					displayed = r.Default
					return "", nil
				default:
					return "", errors.New("unexpected request")
				}
			}
			check := func(_ context.Context, cfg preflight.Config) (preflight.Report, error) {
				if cfg != want {
					return preflight.Report{}, errors.New("incorrect inputs")
				}
				return preflight.Report{Status: "DIFFERENCES", Text: "Missing closer"}, nil
			}
			if err := run(context.Background(), ask, check); err != nil || displayed != "Missing closer" {
				t.Fatalf("report = %q, error = %v", displayed, err)
			}
		})
	}
}

func TestCancelStopsRunningCheck(t *testing.T) {
	_, err := checkWithProgress(context.Background(), func(_ context.Context, r request) (string, error) {
		return "", errCanceled
	}, func(ctx context.Context, _ preflight.Config) (preflight.Report, error) {
		<-ctx.Done()
		return preflight.Report{}, ctx.Err()
	}, preflight.Config{})
	if !errors.Is(err, errCanceled) {
		t.Fatalf("got %v", err)
	}
}
