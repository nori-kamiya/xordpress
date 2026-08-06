package theme

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// assetsDirName is the subdirectory of the active theme root that holds
// static assets served under GET /assets/ (see design.md §5).
const assetsDirName = "assets"

// ResolveAsset resolves name to a file under the active theme's assets/
// directory (see themeFS: on-disk theme preferred, embedded starter theme
// as fallback). name is joined under assets/ and cleaned with
// filepath.Clean; if the cleaned result does not stay within the assets/
// prefix, the asset is rejected. This guards both the on-disk and embedded
// theme roots against path traversal (design.md §5, §7).
func (r *Renderer) ResolveAsset(name string) (fs.File, error) {
	root, err := r.themeFS()
	if err != nil {
		return nil, err
	}

	joined := filepath.ToSlash(filepath.Join(assetsDirName, name))
	cleaned := filepath.ToSlash(filepath.Clean(joined))

	if cleaned != assetsDirName && !strings.HasPrefix(cleaned, assetsDirName+"/") {
		return nil, fmt.Errorf("theme: asset %q resolves outside %s/", name, assetsDirName)
	}
	if cleaned == assetsDirName {
		return nil, fmt.Errorf("theme: asset %q is not a file", name)
	}

	f, err := root.Open(cleaned)
	if err != nil {
		return nil, fmt.Errorf("theme: open asset %q: %w", name, err)
	}

	return f, nil
}
