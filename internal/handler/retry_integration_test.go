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

func TestRetryFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := service.NewInMemoryImportService()
	wp := worker.NewPool(svc, 10, 1)
	h := NewImportHandler(svc, wp)

	router := gin.New()
	api := router.Group("/api/v1")
	api.POST("/imports", h.CreateImport)
	api.POST("/imports/:id/retry", h.RetryFailed)
	api.GET("/imports/:id", h.GetImport)

	// create temp CSV with two bad records and one good
	tmp := filepath.Join(os.TempDir(), "retry_integ_test.csv")
	_ = os.WriteFile(tmp, []byte("code,discount_type,discount_value,expires_at,max_uses\nBAD1,invalid,10,2099-01-01T00:00:00Z,1\nBAD2,invalid,10,2099-01-01T00:00:00Z,1\nGOOD,fixed,5,2099-01-01T00:00:00Z,0\n"), 0644)
	defer os.Remove(tmp)

	// upload file
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
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]interface{})
	origID := data["import_job_id"].(string)

	// wait for original job to complete
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rr := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/imports/"+origID, nil)
		router.ServeHTTP(rr, r)
		if rr.Code == http.StatusOK {
			var jr map[string]interface{}
			_ = json.Unmarshal(rr.Body.Bytes(), &jr)
			d := jr["data"].(map[string]interface{})
			status := d["status"].(string)
			if status != "pending" && status != "processing" {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// call retry endpoint
	rr := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/imports/"+origID+"/retry", nil)
	router.ServeHTTP(rr, r)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for retry, got %d", rr.Code)
	}
	var retryResp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &retryResp); err != nil {
		t.Fatal(err)
	}
	d := retryResp["data"].(map[string]interface{})
	newID := d["import_job_id"].(string)

	// wait for retry job to complete
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rr2 := httptest.NewRecorder()
		r2 := httptest.NewRequest("GET", "/api/v1/imports/"+newID, nil)
		router.ServeHTTP(rr2, r2)
		if rr2.Code == http.StatusOK {
			var jr map[string]interface{}
			_ = json.Unmarshal(rr2.Body.Bytes(), &jr)
			d2 := jr["data"].(map[string]interface{})
			status := d2["status"].(string)
			if status != "processing" && status != "pending" {
				// check results
				if int(d2["success_count"].(float64)) >= 0 {
					break
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// final assert: retry job succeeded for records that could be reprocessed
	rr3 := httptest.NewRecorder()
	r3 := httptest.NewRequest("GET", "/api/v1/imports/"+newID, nil)
	router.ServeHTTP(rr3, r3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("expected 200 for new job, got %d", rr3.Code)
	}
	var jr map[string]interface{}
	_ = json.Unmarshal(rr3.Body.Bytes(), &jr)
	d3 := jr["data"].(map[string]interface{})
	// retry should have attempted the failed rows (2), but they will still fail because invalid dtype
	if int(d3["failure_count"].(float64)) < 1 {
		t.Fatalf("expected retry failures >=1, got %v", d3["failure_count"])
	}
}
