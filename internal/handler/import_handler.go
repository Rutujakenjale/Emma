package handler

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"coupon-import/internal/metrics"
	"coupon-import/internal/service"
	"coupon-import/internal/worker"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ImportHandler struct {
	svc service.ImportServiceInterface
	q   *worker.Pool
}

func NewImportHandler(svc service.ImportServiceInterface, q *worker.Pool) *ImportHandler {
	return &ImportHandler{svc: svc, q: q}
}

func (h *ImportHandler) CreateImport(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "MISSING_FILE", "message": "No file uploaded"}})
		return
	}
	// simple validation
	if filepath.Ext(file.Filename) != ".csv" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "INVALID_FORMAT", "message": "File must be CSV"}})
		return
	}
	// limit upload size to 10MB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	tmpDir := os.TempDir()
	// sanitize filename and use a random prefix to avoid collisions and path traversal
	safeName := filepath.Base(file.Filename)
	dst := filepath.Join(tmpDir, uuid.New().String()+"-"+safeName)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "SAVE_FAILED", "message": "failed to save file"}})
		return
	}

	// validate content-type by sniffing first bytes
	f, err := os.Open(dst)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "OPEN_FAILED", "message": "failed to open uploaded file"}})
		return
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	kind := http.DetectContentType(buf[:n])
	if !strings.HasPrefix(kind, "text/") && kind != "application/octet-stream" && kind != "text/csv" {
		// allow CSV-like octet streams too, but reject obvious binaries
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "INVALID_MIME", "message": "uploaded file is not a valid text/csv"}})
		return
	}
	// validate CSV header
	// read first line
	f.Seek(0, 0)
	first := make([]byte, 2048)
	m, _ := f.Read(first)
	headerLine := string(bytes.SplitN(first[:m], []byte("\n"), 2)[0])
	headerLine = strings.TrimSpace(headerLine)
	expected := "code,discount_type,discount_value,expires_at,max_uses"
	if strings.ToLower(headerLine) != expected {
		// allow some tolerance: CSV header might have spaces
		if strings.ToLower(strings.ReplaceAll(headerLine, " ", "")) != expected {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "INVALID_CSV_HEADER", "message": "invalid CSV header; expected: code,discount_type,discount_value,expires_at,max_uses"}})
			return
		}
	}
	job, err := h.svc.CreateJob(file.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "JOB_CREATE_FAILED", "message": err.Error()}})
		return
	}

	// enqueue job to worker pool
	if h.q != nil {
		ok := h.q.Enqueue(worker.Job{ID: job.ID, Path: dst})
		if !ok {
			// fallback to background goroutine if queue is full
			metrics.JobAccepted.Inc()
			go func(id, path string) {
				if err := h.svc.ProcessFile(id, path); err != nil {
					metrics.JobFailed.Inc()
					log.Printf("background process error for job %s: %v", id, err)
				} else {
					metrics.JobProcessed.Inc()
				}
			}(job.ID, dst)
		}
	} else {
		metrics.JobAccepted.Inc()
		go func(id, path string) {
			if err := h.svc.ProcessFile(id, path); err != nil {
				metrics.JobFailed.Inc()
				log.Printf("background process error for job %s: %v", id, err)
			} else {
				metrics.JobProcessed.Inc()
			}
		}(job.ID, dst)
	}

	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{"import_job_id": job.ID, "file_name": job.FileName, "status": job.Status}})
}

func (h *ImportHandler) GetImport(c *gin.Context) {
	id := c.Param("id")
	job, err := h.svc.GetJob(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "IMPORT_NOT_FOUND", "message": "Import job not found"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": job})
}

func (h *ImportHandler) GetErrors(c *gin.Context) {
	id := c.Param("id")
	rows, err := h.svc.ListErrors(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "ERROR_FETCH", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"import_job_id": id, "total_errors": len(rows), "errors": rows}})
}

func (h *ImportHandler) RetryFailed(c *gin.Context) {
	orig := c.Param("id")
	// create new job
	newJob, err := h.svc.CreateJob("retry-" + orig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "JOB_CREATE_FAILED", "message": err.Error()}})
		return
	}
	go func(origID, newID string) {
		if err := h.svc.RetryFailed(origID, newID); err != nil {
			log.Printf("retry failed job error orig=%s new=%s: %v", origID, newID, err)
		}
	}(orig, newJob.ID)
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{"import_job_id": newJob.ID, "original_job_id": orig, "file_name": newJob.FileName, "status": newJob.Status}})
}
