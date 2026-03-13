package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"coupon-import/internal/service"
	"coupon-import/internal/worker"

	"github.com/gin-gonic/gin"
)

func TestAPIImportFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := service.NewInMemoryImportService()
	wp := worker.NewPool(svc, 10, 1)
	h := NewImportHandler(svc, wp)

	router := gin.New()
	api := router.Group("/api/v1")
	api.POST("/imports", h.CreateImport)
	api.GET("/imports/:id", h.GetImport)
	api.GET("/imports/:id/errors", h.GetErrors)

	// create temp CSV with one bad record
	tmp := filepath.Join(os.TempDir(), "api_integ_test.csv")
	_ = os.WriteFile(tmp, []byte("code,discount_type,discount_value,expires_at,max_uses\nGOOD1,percentage,10,2099-01-01T00:00:00Z,1\nBAD,invalid,10,2099-01-01T00:00:00Z,1\nGOOD2,fixed,5,2099-01-01T00:00:00Z,0\n"), 0644)
	defer os.Remove(tmp)

	// build multipart request
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

	req := httptest.NewRequest("POST", "/api/v1/imports", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}

	// parse response to get job id
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data in response")
	}
	id, ok := data["import_job_id"].(string)
	if !ok || id == "" {
		t.Fatalf("missing import_job_id")
	}

	// poll job status until not pending (with timeout)
	var jobResp map[string]interface{}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rr := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/imports/"+id, nil)
		router.ServeHTTP(rr, r)
		if rr.Code == http.StatusOK {
			if err := json.Unmarshal(rr.Body.Bytes(), &jobResp); err == nil {
				d, _ := jobResp["data"].(map[string]interface{})
				if d != nil {
					status, _ := d["status"].(string)
					if status != "pending" {
						break
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// final check
	rr := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/imports/"+id, nil)
	router.ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 getting job, got %d", rr.Code)
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &jobResp); err != nil {
		t.Fatalf("invalid job json: %v", err)
	}
	d, _ := jobResp["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("missing job data")
	}
	// check counts
	if int(d["total_rows"].(float64)) != 3 {
		t.Fatalf("expected total_rows 3, got %v", d["total_rows"])
	}
	if int(d["success_count"].(float64)) != 2 {
		t.Fatalf("expected success_count 2, got %v", d["success_count"])
	}
	if int(d["failure_count"].(float64)) != 1 {
		t.Fatalf("expected failure_count 1, got %v", d["failure_count"])
	}

	// get errors
	rr = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/api/v1/imports/"+id+"/errors", nil)
	router.ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 errors, got %d", rr.Code)
	}
	var errResp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("invalid errors json: %v", err)
	}
	ed, _ := errResp["data"].(map[string]interface{})
	total := int(ed["total_errors"].(float64))
	if total != 1 {
		t.Fatalf("expected 1 error, got %d", total)
	}
}
