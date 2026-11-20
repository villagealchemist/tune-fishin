package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCommandExitCodes(t *testing.T) {
	root := t.TempDir()
	set := filepath.Join(root, "Friday.m3u8")
	if err := os.WriteFile(set, []byte("opener.wav\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := []string{"check", "--usb", root, "--deck", "CDJ-3000", "--set", set}
	var out, stderr bytes.Buffer
	if code := run(context.Background(), cmd, &out, &stderr); code != 1 || !strings.Contains(out.String(), "DIFFERENCES | Friday") {
		t.Fatalf("missing library: exit %d, stdout %q, stderr %q", code, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	cmd[len(cmd)-1] = set + ".absent"
	if code := run(context.Background(), cmd, &out, &stderr); code != 2 || !strings.Contains(stderr.String(), "INCOMPLETE") {
		t.Fatalf("unreadable set: exit %d, stdout %q, stderr %q", code, out.String(), stderr.String())
	}
}
