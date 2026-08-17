// Package media provides validation, re-encoding, and safe path resolution
// for uploaded media files. It has no HTTP or DB dependencies — it is pure
// logic, unit-testable in isolation.
package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

// Kind identifies a supported image format.
type Kind int

const (
	KindUnknown Kind = iota
	KindJPEG
	KindPNG
	KindGIF
)

var (
	ErrUnsupportedType = errors.New("media: unsupported or unrecognized file type")
	ErrDecodeFailed    = errors.New("media: failed to decode image")
	ErrPathTraversal   = errors.New("media: path resolves outside media root")
)

// Validate checks that declaredMIME is an allowed image type and that the
// sniffed content type of header (the first bytes of the uploaded file)
// matches the declared type. It does not attempt to fully decode the image;
// that is Reencode's job. Validate is a cheap pre-filter, not the sole line
// of defense.
func Validate(declaredMIME string, header []byte) (Kind, error) {
	declaredKind := kindFromMIME(declaredMIME)
	if declaredKind == KindUnknown {
		return KindUnknown, ErrUnsupportedType
	}

	sniffed := http.DetectContentType(header)
	sniffedKind := kindFromMIME(sniffed)
	if sniffedKind == KindUnknown || sniffedKind != declaredKind {
		return KindUnknown, ErrUnsupportedType
	}

	return declaredKind, nil
}

// kindFromMIME maps an exact MIME type string to a Kind, or KindUnknown if
// it is not one of the three allowed image types.
func kindFromMIME(mime string) Kind {
	switch mime {
	case "image/jpeg":
		return KindJPEG
	case "image/png":
		return KindPNG
	case "image/gif":
		return KindGIF
	default:
		return KindUnknown
	}
}

// Reencode decodes the image read from r and re-encodes it according to
// kind, stripping embedded metadata and any malicious payloads that are not
// part of a genuinely well-formed image. kind is trusted as given by the
// caller (typically the result of Validate) — this function does not
// re-sniff the input.
func Reencode(kind Kind, r io.Reader) (data []byte, mimeType string, width, height int, err error) {
	if kind != KindJPEG && kind != KindPNG && kind != KindGIF {
		return nil, "", 0, 0, ErrUnsupportedType
	}

	img, _, decodeErr := image.Decode(r)
	if decodeErr != nil {
		return nil, "", 0, 0, fmt.Errorf("media: decode: %w", ErrDecodeFailed)
	}

	bounds := img.Bounds()
	width = bounds.Dx()
	height = bounds.Dy()

	var buf bytes.Buffer
	switch kind {
	case KindJPEG:
		if encErr := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); encErr != nil {
			return nil, "", 0, 0, fmt.Errorf("media: encode jpeg: %w", encErr)
		}
		mimeType = "image/jpeg"
	case KindPNG:
		if encErr := png.Encode(&buf, img); encErr != nil {
			return nil, "", 0, 0, fmt.Errorf("media: encode png: %w", encErr)
		}
		mimeType = "image/png"
	case KindGIF:
		if encErr := gif.Encode(&buf, img, nil); encErr != nil {
			return nil, "", 0, 0, fmt.Errorf("media: encode gif: %w", encErr)
		}
		mimeType = "image/gif"
	}

	return buf.Bytes(), mimeType, width, height, nil
}

// ResolveMedia resolves storedName to a path under dataDir/media, rejecting
// any path that would escape that root. It mirrors the technique used by
// theme.ResolveAsset, adapted for a real OS filesystem: it returns a
// resolved string path for the caller to os.Open, and does not itself open
// the file, check that it exists, or create the media directory.
func ResolveMedia(dataDir, storedName string) (string, error) {
	root := filepath.Join(dataDir, "media")
	joined := filepath.Join(root, storedName)
	cleaned := filepath.Clean(joined)

	if cleaned != root && !strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
		return "", fmt.Errorf("media: %q resolves outside media root: %w", storedName, ErrPathTraversal)
	}
	if cleaned == root {
		return "", fmt.Errorf("media: %q is not a file: %w", storedName, ErrPathTraversal)
	}

	return cleaned, nil
}
