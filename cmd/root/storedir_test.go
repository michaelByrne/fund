package root

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	live  = true
	local = false
)

// The store directory is where a week of webhook events lives, and getting it
// wrong is silent: everything works until a deploy replaces the container, and
// then the events are gone. Production refuses to start rather than run in that
// state, because a container that will not boot cannot be mistaken for a working
// one.
func TestResolveStoreDir(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("uses a writable configured directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nats")

		got, err := resolveStoreDir(dir, live, quiet)
		if err != nil {
			t.Fatalf("resolveStoreDir: %v", err)
		}

		if got != dir {
			t.Errorf("resolveStoreDir = %q, want %q", got, dir)
		}

		// Created rather than merely accepted: the volume mounts empty.
		if _, err = os.Stat(dir); err != nil {
			t.Errorf("directory should have been created: %v", err)
		}
	})

	t.Run("refuses to start in production with no store dir", func(t *testing.T) {
		_, err := resolveStoreDir("", live, quiet)
		if err == nil {
			t.Fatal("production must not boot without a volume")
		}

		if !strings.Contains(err.Error(), "NATS_STORE_DIR") {
			t.Errorf("the error should name the variable to set, got: %v", err)
		}
	})

	t.Run("allows an unset store dir outside production", func(t *testing.T) {
		// A developer running this locally has no volume and no events worth
		// keeping, and should not need to invent a path to start the server.
		got, err := resolveStoreDir("", local, quiet)
		if err != nil {
			t.Fatalf("local runs should not require a volume: %v", err)
		}

		if !strings.HasPrefix(got, os.TempDir()) {
			t.Errorf("fallback %q should be under the temp dir", got)
		}
	})

	t.Run("refuses a configured path it cannot write to", func(t *testing.T) {
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

		// Refused even outside production: naming a path and having it silently
		// ignored is worse than being told it does not work.
		if _, err := resolveStoreDir(filepath.Join(parent, "nats"), local, quiet); err == nil {
			t.Error("an unwritable store dir was accepted")
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
