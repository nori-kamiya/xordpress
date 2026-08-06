// Package theme renders public-facing pages using the Liquid templating
// language (ADR 0003). It implements the two-pass rendering scheme described
// in design.md §6: a body template ("index" or "post") is rendered first,
// and the resulting string is bound as content_for_layout into the layout
// template.
//
// theme must not import storage — callers (httpapi) are responsible for
// fetching any data (e.g. site settings via content.GetSiteSettings) and
// passing it in as bindings.
package theme

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/osteele/liquid"
)

//go:embed themes/default
var embeddedThemes embed.FS

const (
	defaultThemeName = "default"
	layoutTemplate   = "layout/theme.liquid"
	contentForLayout = "content_for_layout"
	templatesDirName = "templates"
)

// Renderer renders theme templates. DataDir is the base data directory
// (config.DataDir); an on-disk theme at
// filepath.Join(DataDir, "themes", "default") is preferred over the
// embedded starter theme when present.
type Renderer struct {
	DataDir string

	engine *liquid.Engine
}

// New creates a Renderer rooted at dataDir.
func New(dataDir string) *Renderer {
	return &Renderer{
		DataDir: dataDir,
		engine:  liquid.NewEngine(),
	}
}

// Render renders tmplName (e.g. "index" or "post") from
// themes/default/templates/<tmplName>.liquid using data as Liquid bindings,
// then binds the resulting string as content_for_layout and renders
// themes/default/layout/theme.liquid with the same bindings.
func (r *Renderer) Render(ctx context.Context, tmplName string, data any) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	bindings, err := toBindings(data)
	if err != nil {
		return "", err
	}

	themeFS, err := r.themeFS()
	if err != nil {
		return "", err
	}

	engine := r.engine
	if engine == nil {
		engine = liquid.NewEngine()
	}

	bodySrc, err := fs.ReadFile(themeFS, filepath.ToSlash(filepath.Join(templatesDirName, tmplName+".liquid")))
	if err != nil {
		return "", fmt.Errorf("theme: read template %q: %w", tmplName, err)
	}

	body, srcErr := engine.ParseAndRenderString(string(bodySrc), liquid.Bindings(bindings))
	if srcErr != nil {
		return "", fmt.Errorf("theme: render template %q: %w", tmplName, srcErr)
	}

	layoutSrc, err := fs.ReadFile(themeFS, layoutTemplate)
	if err != nil {
		return "", fmt.Errorf("theme: read layout: %w", err)
	}

	layoutBindings := make(liquid.Bindings, len(bindings)+1)
	for k, v := range bindings {
		layoutBindings[k] = v
	}
	layoutBindings[contentForLayout] = body

	out, srcErr := engine.ParseAndRenderString(string(layoutSrc), layoutBindings)
	if srcErr != nil {
		return "", fmt.Errorf("theme: render layout: %w", srcErr)
	}

	return out, nil
}

// themeFS resolves the active theme's filesystem root: an on-disk theme at
// filepath.Join(DataDir, "themes", "default") if present, otherwise the
// embedded starter theme.
func (r *Renderer) themeFS() (fs.FS, error) {
	diskPath := filepath.Join(r.DataDir, "themes", defaultThemeName)
	if info, err := os.Stat(diskPath); err == nil && info.IsDir() {
		return os.DirFS(diskPath), nil
	}

	return fs.Sub(embeddedThemes, filepath.ToSlash(filepath.Join("themes", defaultThemeName)))
}

// toBindings converts data into liquid.Bindings. Only map[string]any (or a
// liquid.Bindings value) is accepted, since it must be merged with
// content_for_layout before the layout pass.
func toBindings(data any) (liquid.Bindings, error) {
	switch v := data.(type) {
	case liquid.Bindings:
		return v, nil
	case map[string]any:
		return liquid.Bindings(v), nil
	case nil:
		return liquid.Bindings{}, nil
	default:
		return nil, fmt.Errorf("theme: data must be map[string]any, got %T", data)
	}
}
