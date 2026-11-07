package snapshot

import (
	"bytes"
	"testing"
)

func TestValidateImageJPEG(t *testing.T) {
	// Build minimal genuine JPEG buffer (> 1024 bytes)
	buf := make([]byte, 2048)
	buf[0] = 0xFF
	buf[1] = 0xD8
	buf[2] = 0xFF
	buf[3] = 0xE0 // APP0 marker
	buf[len(buf)-2] = 0xFF
	buf[len(buf)-1] = 0xD9 // EOI marker

	format, err := ValidateImage(buf)
	if err != nil {
		t.Fatalf("expected valid JPEG, got error: %v", err)
	}
	if format != "JPEG" {
		t.Errorf("expected format JPEG, got %s", format)
	}
}

func TestValidateImagePNG(t *testing.T) {
	buf := make([]byte, 1500)
	copy(buf[:4], []byte{0x89, 0x50, 0x4E, 0x47})

	format, err := ValidateImage(buf)
	if err != nil {
		t.Fatalf("expected valid PNG, got error: %v", err)
	}
	if format != "PNG" {
		t.Errorf("expected format PNG, got %s", format)
	}
}

func TestValidateImageRejectsSVG(t *testing.T) {
	svgContent := `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="640" height="360" fill="#000"/></svg>`
	buf := bytes.Repeat([]byte(svgContent), 20) // make it > 1024 bytes

	_, err := ValidateImage(buf)
	if err == nil {
		t.Fatalf("expected validation error for SVG placeholder, got nil")
	}
	if err != ErrInvalidImageFormat {
		t.Errorf("expected ErrInvalidImageFormat, got %v", err)
	}
}

func TestValidateImageRejectsSmall(t *testing.T) {
	smallBuf := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	_, err := ValidateImage(smallBuf)
	if err != ErrImageTooSmall {
		t.Errorf("expected ErrImageTooSmall, got %v", err)
	}
}
