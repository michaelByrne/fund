package common

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// Assets are served under URLs containing a hash of their contents, so that a
// changed file is a changed URL rather than a stale one.
//
// The origin asks for revalidation on /static/, but Cloudflare rewrites that to
// a four-hour max-age. The result was a stylesheet 55 minutes older than the
// markup that needed it: the archive rows rendered as anchors while the CSS that
// stretches them across the row had not arrived, so only the fund name was
// clickable and the whole row still highlighted. Purging fixes that build and
// nothing after it.
//
// Content addressing makes any TTL correct, ours or anyone else's, because a
// cached copy can only ever be of the bytes its URL names.
type assetSet struct {
	// urls maps a file's path under public/ to the URL that serves it.
	urls map[string]string
	// files maps a fingerprinted request path back to the file on disk. Exact
	// lookup rather than pattern-matching the hash out, so a request can only
	// name a file the server actually hashed.
	files map[string]string
}

var (
	assetsMu sync.RWMutex
	assets   = &assetSet{urls: map[string]string{}, files: map[string]string{}}
)

// LoadAssets hashes everything under dir. Called once at startup: the files are
// baked into the image and cannot change under a running server.
func LoadAssets(dir string) error {
	loaded := &assetSet{urls: map[string]string{}, files: map[string]string{}}

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		contents, err := os.ReadFile(p)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}

		// URL paths, not OS paths, so a Windows build would not emit backslashes.
		rel = filepath.ToSlash(rel)

		sum := sha256.Sum256(contents)
		hashed := fingerprint(rel, hex.EncodeToString(sum[:8]))

		loaded.urls[rel] = "/static/" + hashed
		loaded.files[hashed] = rel

		return nil
	})
	if err != nil {
		return err
	}

	assetsMu.Lock()
	assets = loaded
	assetsMu.Unlock()

	return nil
}

// Asset is the URL to link a file under public/. An unknown name falls back to
// the plain path: a missing hash costs caching, while returning nothing would
// cost the page its stylesheet.
func Asset(name string) string {
	assetsMu.RLock()
	defer assetsMu.RUnlock()

	if url, ok := assets.urls[name]; ok {
		return url
	}

	return "/static/" + name
}

// ResolveAsset maps a requested path back to the file that serves it, reporting
// whether the request named a fingerprinted URL. Only those may be cached
// indefinitely -- a plain path still refers to whatever the file holds today.
func ResolveAsset(requested string) (string, bool) {
	assetsMu.RLock()
	defer assetsMu.RUnlock()

	if file, ok := assets.files[requested]; ok {
		return file, true
	}

	return requested, false
}

// fingerprint puts the hash before the extension, so the served file keeps the
// suffix that decides its content type.
func fingerprint(name, hash string) string {
	ext := path.Ext(name)

	return strings.TrimSuffix(name, ext) + "." + hash + ext
}
