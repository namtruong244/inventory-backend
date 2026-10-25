package service_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"inventory_backend/internal/service"
)

type fakeStorage struct{ uploaded int }

func (f *fakeStorage) Upload(ctx context.Context, file io.Reader, filename, contentType, folder string) (string, error) {
	f.uploaded++
	return "http://x/" + folder + "/" + filename, nil
}

func (f *fakeStorage) Delete(ctx context.Context, fileURL string) error { return nil }

func fileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	w.Close()

	req := httptest.NewRequest("POST", "/", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatal(err)
	}
	return req.MultipartForm.File["file"][0]
}

func TestMediaService_Upload(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	heic := append([]byte{0, 0, 0, 24}, []byte("ftypheic")...)

	cases := []struct {
		name    string
		file    string
		content []byte
		wantErr bool
	}{
		{"valid png", "a.png", png, false},
		{"valid heic", "a.heic", heic, false},
		{"valid pdf", "a.pdf", []byte("%PDF-1.4\n..."), false},
		{"html disguised as png", "a.png", []byte("<html><script>alert(1)</script></html>"), true},
		{"png content with pdf extension", "a.pdf", png, true},
		{"disallowed extension", "a.svg", []byte("<svg/>"), true},
		{"too large", "a.png", append(png, make([]byte, service.MaxUploadSize)...), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &fakeStorage{}
			svc := service.NewMediaService(st)
			_, err := svc.Upload(fileHeader(t, tc.file, tc.content), "items")
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got err=%v", tc.wantErr, err)
			}
			if tc.wantErr && st.uploaded != 0 {
				t.Errorf("rejected file must not reach storage")
			}
		})
	}
}
