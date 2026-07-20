package images

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"

	"Canto/internal/config"
	"Canto/internal/db"
)

// Size names a resize target, or SizeSource for the cached original crop.
type Size string

const (
	SizeSmall  Size = "128x128"
	SizeMedium Size = "300x300"
	SizeLarge  Size = "640x640"
	SizeSource Size = "source"

	cacheDir = "image_cache"
)

// px returns the target square side length in pixels for s.
func (s Size) px() int {
	switch s {
	case SizeSmall:
		return 128
	case SizeMedium:
		return 300
	case SizeLarge:
		return 640
	case SizeSource:
		return 1200
	default:
		return 0
	}
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Download fetches url, center-crops it to a square, and caches it under id at source quality.
func Download(id uuid.UUID, url string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("images: download: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("images: download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("images: download: status %d", resp.StatusCode)
	}

	if err := saveResized(id, SizeSource, resp.Body); err != nil {
		return fmt.Errorf("images: download: %w", err)
	}
	return nil
}

// Store crops and caches an uploaded image under id at source quality, same as Download but from local bytes.
func Store(id uuid.UUID, r io.Reader) error {
	if err := saveResized(id, SizeSource, r); err != nil {
		return fmt.Errorf("images: store: %w", err)
	}
	return nil
}

// Path returns the on-disk path id's cached image at size would live at, downloaded or not.
func Path(id uuid.UUID, size Size) string {
	return filepath.Join(config.DataDir, cacheDir, id.String()[:2], id.String(), string(size)+".jpg")
}

// Get returns the path to id's image at size, resizing and caching it from the source crop on first request.
func Get(id uuid.UUID, size Size) (string, error) {
	target := Path(id, size)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("images: get: %w", err)
	}

	source := Path(id, SizeSource)
	f, err := os.Open(source)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("images: get: %w: %s", fs.ErrNotExist, id)
		}
		return "", fmt.Errorf("images: get: %w", err)
	}
	defer f.Close()

	if err := saveResized(id, size, f); err != nil {
		return "", fmt.Errorf("images: get: %w", err)
	}
	return target, nil
}

// Delete removes every cached size for id.
func Delete(id uuid.UUID) error {
	dir := filepath.Dir(Path(id, SizeSource))
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("images: delete: %w", err)
	}
	return nil
}

// DeleteIfSet removes id's cached image files if it's set, logging and swallowing any failure.
func DeleteIfSet(id pgtype.UUID) {
	u, ok := db.UUID(id)
	if !ok {
		return
	}
	if err := Delete(u); err != nil {
		slog.Warn("images: delete failed", "id", u, "err", err)
	}
}

// saveResized decodes data, center-crops it to a square, resizes it to size, and writes it as JPEG to id's cache path.
func saveResized(id uuid.UUID, size Size, data io.Reader) error {
	img, err := decode(data)
	if err != nil {
		return err
	}

	cropped := centerCropSquare(img)
	px := size.px()
	if px == 0 || px > cropped.Bounds().Dx() {
		px = cropped.Bounds().Dx()
	}

	dst := image.NewRGBA(image.Rect(0, 0, px, px))
	draw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), draw.Over, nil)

	path := Path(id, size)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer f.Close()

	if err := jpeg.Encode(f, dst, &jpeg.Options{Quality: 90}); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

// decode sniffs and decodes an image, falling back to webp when the standard decoders don't recognize it.
func decode(r io.Reader) (image.Image, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(buf))
	if err == nil {
		return img, nil
	}
	if img, werr := webp.Decode(bytes.NewReader(buf)); werr == nil {
		return img, nil
	}
	return nil, fmt.Errorf("decode: %w", err)
}

// centerCropSquare returns the largest centered square crop of img.
func centerCropSquare(img image.Image) image.Image {
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	rect := image.Rect(x0, y0, x0+side, y0+side)

	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
	return dst
}
