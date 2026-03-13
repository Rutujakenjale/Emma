package handler

import (
	"net/http"
	"os"
	"path/filepath"

	"coupon-import/internal/logging"
	"coupon-import/internal/service"

	"github.com/gin-gonic/gin"
)

type ImportHandler struct {
	svc service.ImportServiceInterface
}

func NewImportHandler(svc service.ImportServiceInterface) *ImportHandler {
	return &ImportHandler{svc: svc}
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
	tmpDir := os.TempDir()
	dst := filepath.Join(tmpDir, file.Filename)
	logging.Debugf("CreateImport: saving uploaded file to %s", dst)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		logging.Errorf("CreateImport: save failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "SAVE_FAILED", "message": "failed to save file"}})
		return
	}
	job, err := h.svc.CreateJob(file.Filename)
	if err != nil {
		logging.Errorf("CreateImport: create job failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "JOB_CREATE_FAILED", "message": err.Error()}})
		return
	}

	// start background processing
	logging.Debugf("CreateImport: starting background processing job=%s path=%s", job.ID, dst)
	go func(id, path string) {
		_ = h.svc.ProcessFile(id, path)
	}(job.ID, dst)

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
		_ = h.svc.RetryFailed(origID, newID)
	}(orig, newJob.ID)
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{"import_job_id": newJob.ID, "original_job_id": orig, "file_name": newJob.FileName, "status": newJob.Status}})
}
