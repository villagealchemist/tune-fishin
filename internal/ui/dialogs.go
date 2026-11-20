package ui

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"
)

//go:embed mac.js
var macScript string

//go:embed windows.ps1
var windowsScript string

func newDialogs() (askFunc, error) {
	var binary string
	switch runtime.GOOS {
	case "darwin":
		binary = "/usr/bin/osascript"
	case "windows":
		binary = "powershell.exe"
	case "linux":
		binary = "zenity"
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return nil, fmt.Errorf("a graphical desktop is required. Run tuna check --help for the terminal workflow")
		}
	default:
		return nil, fmt.Errorf("desktop dialogs are unavailable on %s. Run tuna check --help", runtime.GOOS)
	}
	executable, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("desktop dialogs require %s. Run tuna check --help for the terminal workflow", binary)
	}
	return func(ctx context.Context, r request) (string, error) {
		if runtime.GOOS == "linux" {
			return zenity(ctx, executable, r)
		}
		// JSON on standard input keeps paths and arbitrary playlist text out of
		// shell/PowerShell source, command-line limits and process listings.
		input, err := json.Marshal(r)
		if err != nil {
			return "", err
		}
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.CommandContext(ctx, executable, "-l", "JavaScript", "-e", macScript)
		} else {
			cmd = exec.CommandContext(ctx, executable, "-NoProfile", "-NonInteractive", "-STA",
				"-EncodedCommand", encodeCommand(windowsScript))
		}
		cmd.Stdin = bytes.NewReader(input)
		output, err := cmd.Output()
		if err != nil {
			return "", dialogError(ctx, err)
		}
		var response struct {
			Value    string `json:"value"`
			Canceled bool   `json:"canceled"`
		}
		if err := json.Unmarshal(output, &response); err != nil {
			return "", fmt.Errorf("could not read the desktop dialog response: %w", err)
		}
		if response.Canceled {
			return "", errCanceled
		}
		return response.Value, nil
	}, nil
}

func encodeCommand(script string) string {
	units := utf16.Encode([]rune(script))
	data := make([]byte, 0, 2*len(units))
	for _, c := range units {
		data = append(data, byte(c), byte(c>>8))
	}
	return base64.StdEncoding.EncodeToString(data)
}

func zenity(ctx context.Context, executable string, r request) (string, error) {
	args := []string{"--title", title}
	switch r.Kind {
	case "choice":
		args = append(args, "--list", "--text", r.Prompt, "--column", "Select", "--height", "400", "--width", "530")
		args = append(args, r.Choices...)
	case "file", "folder":
		args = []string{"--file-selection", "--title", r.Prompt}
		if r.Kind == "folder" {
			args = append(args, "--directory")
		} else {
			args = append(args, "--file-filter", "Playlists | *.m3u8 *.m3u *.M3U8 *.M3U", "--file-filter", "All files | *")
		}
	case "entry":
		args = append(args, "--entry", "--text", r.Prompt, "--entry-text", r.Default)
	case "busy":
		args = append(args, "--info", "--text", r.Prompt, "--ok-label", "Cancel check")
	case "report":
		args = []string{"--text-info", "--title", title + " · " + r.Prompt, "--width", "800", "--height", "540"}
	default:
		return "", fmt.Errorf("unknown dialog %q", r.Kind)
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	if r.Kind == "report" {
		cmd.Stdin = strings.NewReader(r.Default)
	}
	output, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if ctx.Err() == nil && errors.As(err, &exit) && exit.ExitCode() == 1 && len(bytes.TrimSpace(exit.Stderr)) == 0 {
			return "", errCanceled
		}
		return "", dialogError(ctx, err)
	}
	return strings.TrimSuffix(string(output), "\n"), nil
}

func dialogError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && len(bytes.TrimSpace(exit.Stderr)) != 0 {
		return fmt.Errorf("desktop dialog failed (%s). Run tuna check --help", strings.TrimSpace(string(exit.Stderr)))
	}
	return fmt.Errorf("desktop dialog failed: %w. Run tuna check --help", err)
}
