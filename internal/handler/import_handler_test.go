package handler

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"coupon-import/internal/service"

	"github.com/gin-gonic/gin"
)

func TestCreateImportHandler(t *testing.T) {
	svc := service.NewInMemoryImportService()
	h := NewImportHandler(svc)

	// create sample CSV file
	tmp := filepath.Join(os.TempDir(), "handler_test.csv")
	os.WriteFile(tmp, []byte("code,discount_type,discount_value,expires_at,max_uses\nX1,percentage,10,2099-01-01T00:00:00Z,1\n"), 0644)
	defer os.Remove(tmp)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(tmp))
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(tmp)
	defer f.Close()
	_, _ = io.Copy(part, f)
	writer.Close()

	router := gin.New()
	router.POST("/api/v1/imports", h.CreateImport)

	req := httptest.NewRequest("POST", "/api/v1/imports", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", w.Code)
	}
}
