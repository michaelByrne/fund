package root

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The store directory is where a week of webhook events lives. Getting it wrong
// is silent -- everything works until a deploy replaces the container, and then
// the events are gone -- so the two ways of getting it wrong are checked here
// rather than discovered in production.
func TestResolveStoreDir(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("uses a writable configured directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nats")

		if got := resolveStoreDir(dir, quiet); got != dir {
			t.Errorf("resolveStoreDir = %q, want %q", got, dir)
		}

		// Created rather than merely accepted: the volume mounts empty.
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("directory should have been created: %v", err)
		}
	})

	t.Run("falls back when unset", func(t *testing.T) {
		got := resolveStoreDir("", quiet)

		if got == "" {
			t.Fatal("an unset store dir must still yield somewhere to write")
		}

		if !strings.HasPrefix(got, os.TempDir()) {
			t.Errorf("fallback %q should be under the temp dir", got)
		}
	})

	t.Run("falls back when the path cannot be written to", func(t *testing.T) {
		if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			t.Skip("permissions are checked differently here")
		}

		if os.Geteuid() == 0 {
			t.Skip("root can write anywhere, which is how the container runs")
		}

		// A directory that exists but is not writable, which is what a volume
		// mounted with the wrong ownership looks like.
		parent := t.TempDir()
		if err := os.Chmod(parent, 0o500); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

		unwritable := filepath.Join(parent, "nats")

		got := resolveStoreDir(unwritable, quiet)

		// The important part is that it does not return the bad path, because
		// JetStream would then fail to start and take the whole site with it.
		if got == unwritable {
			t.Error("an unwritable store dir was accepted, so the server would fail to start")
		}
	})
}

// A directory existing is not the same as this process being allowed to write to
// it, and only the second one matters.
func TestCheckWritableRejectsADirectoryItCannotWrite(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}

	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	if err := checkWritable(filepath.Join(parent, "nats")); err == nil {
		t.Error("expected an error for a directory that cannot be created")
	}

	if err := checkWritable(t.TempDir()); err != nil {
		t.Errorf("a writable directory should pass: %v", err)
	}
}
