package theme

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRender_IndexComposesIntoLayoutWithSiteName(t *testing.T) {
	r := New(t.TempDir())

	data := map[string]any{
		"site": map[string]any{"name": "My XordPress Site"},
		"posts": []map[string]any{
			{"slug": "hello-world", "title": "Hello World", "body": "First post body"},
		},
	}

	out, err := r.Render(context.Background(), "index", data)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if !strings.Contains(out, "<title>My XordPress Site</title>") {
		t.Errorf("expected layout <title> to contain site.name, got: %s", out)
	}
	if !strings.Contains(out, "Hello World") {
		t.Errorf("expected rendered index body (post title) in output, got: %s", out)
	}
	if !strings.Contains(out, "First post body") {
		t.Errorf("expected rendered index body (post body) in output, got: %s", out)
	}
	if !strings.Contains(out, `href="/hello-world"`) {
		t.Errorf("expected post link in output, got: %s", out)
	}
	// Layout chrome (from theme.liquid) must be present too — confirms the
	// two-pass composition actually happened, not just body rendering.
	if !strings.Contains(out, `<html`) {
		t.Errorf("expected layout HTML wrapper in output, got: %s", out)
	}
}

// Regression test for a confirmed vulnerability (roadmap-to-wp-parity.md
// P0-1): the liquid engine did not auto-escape {{ }} output, so a post
// titled "<script>...</script>" was served verbatim as live script on the
// public site. post.title (plain text) must now be escaped; post.body
// (intentionally raw HTML per the posts table schema) must still pass
// through unescaped via the explicit `| safe` filter in the templates.
func TestRender_EscapesTitleButNotBody(t *testing.T) {
	r := New(t.TempDir())

	data := map[string]any{
		"site": map[string]any{"name": "Escape Test Site"},
		"post": map[string]any{
			"slug":  "escape-test",
			"title": `5 < 10 & <script>alert(1)</script>`,
			"body":  `<strong>raw html by design</strong>`,
		},
	}

	out, err := r.Render(context.Background(), "post", data)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Errorf("post.title was not escaped — raw <script> tag reached the served HTML: %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("expected post.title's <script> tag to be HTML-escaped, got: %s", out)
	}
	if !strings.Contains(out, "5 &lt; 10 &amp; ") {
		t.Errorf("expected post.title's < and & to be HTML-escaped, got: %s", out)
	}
	if !strings.Contains(out, "<strong>raw html by design</strong>") {
		t.Errorf("expected post.body to pass through unescaped (raw HTML by design), got: %s", out)
	}
}

func TestRender_PostComposesIntoLayout(t *testing.T) {
	r := New(t.TempDir())

	data := map[string]any{
		"site": map[string]any{"name": "Another Site"},
		"post": map[string]any{"slug": "my-post", "title": "My Post Title", "body": "My post body content"},
	}

	out, err := r.Render(context.Background(), "post", data)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if !strings.Contains(out, "<title>Another Site</title>") {
		t.Errorf("expected layout <title> to contain site.name, got: %s", out)
	}
	if !strings.Contains(out, "My Post Title") {
		t.Errorf("expected post title in output, got: %s", out)
	}
	if !strings.Contains(out, "My post body content") {
		t.Errorf("expected post body in output, got: %s", out)
	}
}

func TestRender_UnknownTemplateReturnsError(t *testing.T) {
	r := New(t.TempDir())

	_, err := r.Render(context.Background(), "does-not-exist", map[string]any{})
	if err == nil {
		t.Fatal("expected an error for an unknown template name, got nil")
	}
}

func TestRender_PrefersOnDiskThemeOverEmbedded(t *testing.T) {
	dataDir := t.TempDir()
	themeDir := filepath.Join(dataDir, "themes", "default")
	if err := os.MkdirAll(filepath.Join(themeDir, "layout"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(themeDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(themeDir, "layout", "theme.liquid"),
		[]byte("CUSTOM-LAYOUT:{{ content_for_layout }}"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(themeDir, "templates", "index.liquid"),
		[]byte("CUSTOM-INDEX"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	r := New(dataDir)
	out, err := r.Render(context.Background(), "index", map[string]any{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if out != "CUSTOM-LAYOUT:CUSTOM-INDEX" {
		t.Errorf("expected on-disk theme to take precedence over embedded theme, got: %q", out)
	}
}

func TestRender_FallsBackToEmbeddedThemeWhenNoDiskOverride(t *testing.T) {
	// dataDir has no themes/default directory at all.
	r := New(t.TempDir())

	out, err := r.Render(context.Background(), "index", map[string]any{"site": map[string]any{"name": "Embedded Fallback"}})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if !strings.Contains(out, "<title>Embedded Fallback</title>") {
		t.Errorf("expected embedded theme layout to render, got: %s", out)
	}
}
