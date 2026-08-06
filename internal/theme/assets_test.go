package theme

import (
	"io"
	"net/url"
	"strings"
	"testing"
)

func TestResolveAsset_ValidAssetIsServed(t *testing.T) {
	r := New(t.TempDir())

	f, err := r.ResolveAsset("style.css")
	if err != nil {
		t.Fatalf("ResolveAsset(%q) returned error: %v", "style.css", err)
	}
	defer f.Close()

	body, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("reading resolved asset: %v", err)
	}
	if !strings.Contains(string(body), "font-family") {
		t.Errorf("expected style.css content, got: %s", body)
	}
}

func TestResolveAsset_RejectsPathTraversal(t *testing.T) {
	r := New(t.TempDir())

	// design.md §5: a raw "../" is normalized/redirected by net/http's
	// ServeMux before it ever reaches the handler, so the guard that
	// actually matters in production is exercised via a percent-encoded
	// traversal string. Decode it here the same way the HTTP layer would
	// before the (still-undecoded-at-the-guard) name reaches ResolveAsset,
	// and confirm the traversal attempt is rejected.
	const encoded = "%2e%2e%2f%2e%2e%2fetc%2fpasswd"
	decoded, err := url.PathUnescape(encoded)
	if err != nil {
		t.Fatalf("PathUnescape(%q): %v", encoded, err)
	}
	if !strings.Contains(decoded, "..") {
		t.Fatalf("test setup error: decoded traversal string %q does not contain \"..\"", decoded)
	}

	if _, err := r.ResolveAsset(decoded); err == nil {
		t.Fatalf("ResolveAsset(%q) succeeded, want error rejecting path traversal", decoded)
	}
}

func TestResolveAsset_RejectsPlainTraversal(t *testing.T) {
	r := New(t.TempDir())

	if _, err := r.ResolveAsset("../../../../etc/passwd"); err == nil {
		t.Fatal("ResolveAsset succeeded for a plain \"../\" traversal, want error")
	}
}
