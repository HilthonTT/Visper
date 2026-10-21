package storage

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fileHeader builds a *multipart.FileHeader the way gin's ctx.FormFile does.
func fileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = req.MultipartForm.RemoveAll() })

	return req.MultipartForm.File["file"][0]
}

func TestResolveRejectsTraversal(t *testing.T) {
	s, err := NewLocalStorageAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path string
		ok   bool
	}{
		{"room/file.png", true},
		{"/room/file.png", true},
		{"../secret.yml", false},
		{"room/../../secret.yml", false},
		{"/../../etc/passwd", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			_, err := s.GetFilePath(tt.path)
			if (err == nil) != tt.ok {
				t.Fatalf("GetFilePath(%q) err = %v, want ok=%v", tt.path, err, tt.ok)
			}
			if !tt.ok && !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("GetFilePath(%q) err = %v, want ErrInvalidPath", tt.path, err)
			}
		})
	}
}

func TestFileExistsDoesNotEscapeBase(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "uploads")
	s, err := NewLocalStorageAt(base)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "secret.yml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if s.FileExists("../secret.yml") {
		t.Fatal("FileExists must not see files outside the uploads directory")
	}
}

func TestSaveFile(t *testing.T) {
	s, err := NewLocalStorageAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		roomID   string
		filename string
		content  []byte
		wantErr  error
	}{
		{"valid png", "room-1", "photo.PNG", pngBytes(t), nil},
		{"html disguised as png", "room-1", "x.png", []byte("<html><script>alert(1)</script></html>"), ErrInvalidFileType},
		{"disallowed extension", "room-1", "x.svg", pngBytes(t), ErrInvalidFileType},
		{"room traversal", "../escape", "photo.png", pngBytes(t), ErrInvalidPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel, id, mime, err := s.SaveFile(fileHeader(t, tt.filename, tt.content), tt.roomID)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SaveFile err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SaveFile: %v", err)
			}

			if mime != "image/png" {
				t.Errorf("mime = %q, want image/png", mime)
			}
			if want := tt.roomID + "/" + id + ".png"; rel != want {
				t.Errorf("relative path = %q, want %q", rel, want)
			}
			if strings.Contains(rel, `\`) {
				t.Errorf("relative path %q must use forward slashes (it is used in URLs)", rel)
			}
			if !s.FileExists(rel) {
				t.Fatalf("saved file %q does not exist", rel)
			}

			if err := s.DeleteFile(rel); err != nil {
				t.Fatalf("DeleteFile: %v", err)
			}
			if s.FileExists(rel) {
				t.Fatal("file still exists after DeleteFile")
			}
		})
	}
}
