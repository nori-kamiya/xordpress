package media

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"testing"
)

// makeJPEG returns bytes for a small, valid JPEG image.
func makeJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 10), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("failed to build test jpeg: %v", err)
	}
	return buf.Bytes()
}

// makePNG returns bytes for a small, valid PNG image.
func makePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 10), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to build test png: %v", err)
	}
	return buf.Bytes()
}

// makeGIF returns bytes for a small, valid GIF image.
func makeGIF(t *testing.T) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{
		color.RGBA{0, 0, 0, 255},
		color.RGBA{255, 255, 255, 255},
	})
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("failed to build test gif: %v", err)
	}
	return buf.Bytes()
}

func header(data []byte) []byte {
	if len(data) > 512 {
		return data[:512]
	}
	return data
}

func TestValidate(t *testing.T) {
	jpegBytes := makeJPEG(t)
	pngBytes := makePNG(t)
	gifBytes := makeGIF(t)

	t.Run("valid jpeg", func(t *testing.T) {
		kind, err := Validate("image/jpeg", header(jpegBytes))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kind != KindJPEG {
			t.Fatalf("expected KindJPEG, got %v", kind)
		}
	})

	t.Run("valid png", func(t *testing.T) {
		kind, err := Validate("image/png", header(pngBytes))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kind != KindPNG {
			t.Fatalf("expected KindPNG, got %v", kind)
		}
	})

	t.Run("valid gif", func(t *testing.T) {
		kind, err := Validate("image/gif", header(gifBytes))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kind != KindGIF {
			t.Fatalf("expected KindGIF, got %v", kind)
		}
	})

	t.Run("webp declared type rejected", func(t *testing.T) {
		_, err := Validate("image/webp", header(pngBytes))
		if !errors.Is(err, ErrUnsupportedType) {
			t.Fatalf("expected ErrUnsupportedType, got %v", err)
		}
	})

	t.Run("declared jpeg but actually png (spoofing)", func(t *testing.T) {
		_, err := Validate("image/jpeg", header(pngBytes))
		if !errors.Is(err, ErrUnsupportedType) {
			t.Fatalf("expected ErrUnsupportedType, got %v", err)
		}
	})

	t.Run("declared jpeg but plaintext payload", func(t *testing.T) {
		_, err := Validate("image/jpeg", header([]byte("<?php system($_GET['c']); ?>")))
		if !errors.Is(err, ErrUnsupportedType) {
			t.Fatalf("expected ErrUnsupportedType, got %v", err)
		}
	})
}

func TestReencode(t *testing.T) {
	t.Run("valid jpeg", func(t *testing.T) {
		data, mimeType, width, height, err := Reencode(KindJPEG, bytes.NewReader(makeJPEG(t)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mimeType != "image/jpeg" {
			t.Fatalf("expected image/jpeg, got %q", mimeType)
		}
		if width <= 0 || height <= 0 {
			t.Fatalf("expected positive dimensions, got %dx%d", width, height)
		}
		if len(data) == 0 {
			t.Fatal("expected non-empty output")
		}
		if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
			t.Fatalf("re-encoded jpeg does not decode: %v", err)
		}
	})

	t.Run("valid png", func(t *testing.T) {
		data, mimeType, width, height, err := Reencode(KindPNG, bytes.NewReader(makePNG(t)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mimeType != "image/png" {
			t.Fatalf("expected image/png, got %q", mimeType)
		}
		if width <= 0 || height <= 0 {
			t.Fatalf("expected positive dimensions, got %dx%d", width, height)
		}
		if len(data) == 0 {
			t.Fatal("expected non-empty output")
		}
		if _, err := png.Decode(bytes.NewReader(data)); err != nil {
			t.Fatalf("re-encoded png does not decode: %v", err)
		}
	})

	t.Run("valid gif", func(t *testing.T) {
		data, mimeType, width, height, err := Reencode(KindGIF, bytes.NewReader(makeGIF(t)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mimeType != "image/gif" {
			t.Fatalf("expected image/gif, got %q", mimeType)
		}
		if width <= 0 || height <= 0 {
			t.Fatalf("expected positive dimensions, got %dx%d", width, height)
		}
		if len(data) == 0 {
			t.Fatal("expected non-empty output")
		}
		if _, err := gif.Decode(bytes.NewReader(data)); err != nil {
			t.Fatalf("re-encoded gif does not decode: %v", err)
		}
	})

	t.Run("garbage bytes fail decode", func(t *testing.T) {
		_, _, _, _, err := Reencode(KindJPEG, bytes.NewReader([]byte("not an image, just garbage bytes")))
		if !errors.Is(err, ErrDecodeFailed) {
			t.Fatalf("expected ErrDecodeFailed, got %v", err)
		}
	})

	t.Run("unknown kind rejected", func(t *testing.T) {
		_, _, _, _, err := Reencode(KindUnknown, bytes.NewReader(makeJPEG(t)))
		if !errors.Is(err, ErrUnsupportedType) {
			t.Fatalf("expected ErrUnsupportedType, got %v", err)
		}
	})
}

func TestResolveMedia(t *testing.T) {
	dataDir := "/data/xordpress"

	t.Run("plain filename resolves under media root", func(t *testing.T) {
		got, err := ResolveMedia(dataDir, "abc.jpg")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(dataDir, "media", "abc.jpg")
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("parent traversal rejected", func(t *testing.T) {
		_, err := ResolveMedia(dataDir, "../../etc/passwd")
		if !errors.Is(err, ErrPathTraversal) {
			t.Fatalf("expected ErrPathTraversal, got %v", err)
		}
	})

	t.Run("sibling directory prefix escape rejected", func(t *testing.T) {
		_, err := ResolveMedia(dataDir, "../media-sibling/x.jpg")
		if !errors.Is(err, ErrPathTraversal) {
			t.Fatalf("expected ErrPathTraversal, got %v", err)
		}
	})

	t.Run("resolving to root itself rejected (dot)", func(t *testing.T) {
		_, err := ResolveMedia(dataDir, ".")
		if !errors.Is(err, ErrPathTraversal) {
			t.Fatalf("expected ErrPathTraversal, got %v", err)
		}
	})

	t.Run("resolving to root itself rejected (empty)", func(t *testing.T) {
		_, err := ResolveMedia(dataDir, "")
		if !errors.Is(err, ErrPathTraversal) {
			t.Fatalf("expected ErrPathTraversal, got %v", err)
		}
	})
}
