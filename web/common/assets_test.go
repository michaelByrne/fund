package common

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAssets(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, contents := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	if err := LoadAssets(dir); err != nil {
		t.Fatalf("load: %v", err)
	}

	return dir
}

func TestChangedContentsMeanAChangedURL(t *testing.T) {
	writeAssets(t, map[string]string{"styles.css": "a{}"})
	before := Asset("styles.css")

	writeAssets(t, map[string]string{"styles.css": "a{color:red}"})
	after := Asset("styles.css")

	// This is the whole point: Cloudflare rewrote our no-cache to a four-hour
	// max-age and served yesterday's stylesheet against today's markup. A cache
	// cannot do that when the bytes it holds are named by the URL.
	if before == after {
		t.Fatal("a changed stylesheet must not reuse its URL")
	}

	// Same bytes, same URL -- otherwise every deploy would evict assets that
	// never changed.
	writeAssets(t, map[string]string{"styles.css": "a{color:red}"})
	if again := Asset("styles.css"); again != after {
		t.Errorf("unchanged contents should keep their URL: %s then %s", after, again)
	}
}

func TestAssetURLKeepsTheExtension(t *testing.T) {
	writeAssets(t, map[string]string{"styles.css": "a{}"})

	url := Asset("styles.css")

	// The extension decides the content type, so the hash goes before it. A URL
	// ending .css.9f86 would be served as something the browser will not apply.
	if !strings.HasSuffix(url, ".css") {
		t.Errorf("URL %q should still end in .css", url)
	}

	if !strings.HasPrefix(url, "/static/styles.") || url == "/static/styles.css" {
		t.Errorf("URL %q should carry a hash", url)
	}
}

func TestResolveAssetOnlyTrustsNamesItHashed(t *testing.T) {
	writeAssets(t, map[string]string{"styles.css": "a{}"})

	hashed := strings.TrimPrefix(Asset("styles.css"), "/static/")

	file, ok := ResolveAsset(hashed)
	if !ok || file != "styles.css" {
		t.Errorf("ResolveAsset(%q) = %q, %v; want styles.css, true", hashed, file, ok)
	}

	// A plain name still means whatever the file holds today, so it must not be
	// reported as fingerprinted -- that flag is what grants a year of caching.
	if _, ok := ResolveAsset("styles.css"); ok {
		t.Error("an unhashed name must not be treated as content-addressed")
	}

	// An invented hash must not resolve. Exact lookup rather than pattern
	// matching is what makes that true.
	if _, ok := ResolveAsset("styles.0000000000000000.css"); ok {
		t.Error("a hash the server never issued must not resolve")
	}
}

func TestUnknownAssetStillGetsAUsableURL(t *testing.T) {
	writeAssets(t, map[string]string{"styles.css": "a{}"})

	// Losing the hash costs caching. Returning nothing would cost the page its
	// stylesheet, which is the worse failure.
	if got := Asset("nope.css"); got != "/static/nope.css" {
		t.Errorf("Asset(nope.css) = %q, want /static/nope.css", got)
	}
}

func TestLayoutLinksTheFingerprintedStylesheet(t *testing.T) {
	// Against the real public/ directory, because the failure this prevents was
	// end to end: markup asking for a URL, a cache answering with older bytes.
	if err := LoadAssets("../../public"); err != nil {
		t.Fatalf("load: %v", err)
	}

	var out strings.Builder
	if err := Head().Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	url := Asset("styles.css")

	if url == "/static/styles.css" {
		t.Fatal("the stylesheet should be fingerprinted")
	}

	if !strings.Contains(out.String(), `href="`+url+`"`) {
		t.Errorf("the layout should link %s", url)
	}

	// And the server must be able to serve back what the layout asks for.
	file, ok := ResolveAsset(strings.TrimPrefix(url, "/static/"))
	if !ok || file != "styles.css" {
		t.Errorf("the linked URL should resolve to styles.css, got %q (%v)", file, ok)
	}
}
