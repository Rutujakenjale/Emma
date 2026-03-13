package service

import (
	"bufio"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"coupon-import/internal/model"

	"github.com/google/uuid"
)

type InMemoryImportService struct {
	mu     sync.RWMutex
	jobs   map[string]*model.ImportJob
	errors map[string][]model.ImportError
	codes  map[string]struct{}
}

func NewInMemoryImportService() *InMemoryImportService {
	return &InMemoryImportService{
		jobs:   make(map[string]*model.ImportJob),
		errors: make(map[string][]model.ImportError),
		codes:  make(map[string]struct{}),
	}
}

func (s *InMemoryImportService) CreateJob(fileName string) (*model.ImportJob, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	j := &model.ImportJob{ID: id, FileName: fileName, Status: "pending", CreatedAt: now}
	s.mu.Lock()
	s.jobs[id] = j
	s.mu.Unlock()
	return j, nil
}

func (s *InMemoryImportService) GetJob(id string) (*model.ImportJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return j, nil
}

func (s *InMemoryImportService) ListErrors(jobID string) ([]model.ImportError, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.errors[jobID], nil
}

func (s *InMemoryImportService) ProcessFile(jobID string, path string) error {
	s.mu.Lock()
	j, ok := s.jobs[jobID]
	if !ok {
		s.mu.Unlock()
		return errors.New("job not found")
	}
	t := time.Now().UTC()
	j.Status = "processing"
	j.StartedAt = &t
	s.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := csv.NewReader(bufio.NewReader(f))
	reader.ReuseRecord = true
	// header
	if _, err := reader.Read(); err != nil {
		return err
	}

	rowNum := 1
	var success, failure, processed int

	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowNum++
		processed++
		if err != nil {
			s.recordError(jobID, rowNum, "", err.Error())
			failure++
			continue
		}
		if len(rec) < 5 {
			s.recordError(jobID, rowNum, strings.Join(rec, ","), "invalid columns")
			failure++
			continue
		}
		code := strings.TrimSpace(rec[0])
		dtype := strings.TrimSpace(rec[1])
		_, _ = strconv.ParseFloat(strings.TrimSpace(rec[2]), 64)
		expires := strings.TrimSpace(rec[3])
		_, _ = strconv.Atoi(strings.TrimSpace(rec[4]))

		if code == "" {
			s.recordError(jobID, rowNum, strings.Join(rec, ","), "code required")
			failure++
			continue
		}
		if dtype != "percentage" && dtype != "fixed" {
			s.recordError(jobID, rowNum, strings.Join(rec, ","), "invalid discount type")
			failure++
			continue
		}
		if _, err := time.Parse(time.RFC3339, expires); err != nil {
			s.recordError(jobID, rowNum, strings.Join(rec, ","), "invalid expires format")
			failure++
			continue
		}
		// uniqueness
		s.mu.Lock()
		if _, exists := s.codes[code]; exists {
			s.mu.Unlock()
			s.recordError(jobID, rowNum, strings.Join(rec, ","), "duplicate code")

			failure++
			continue
		}
		s.codes[code] = struct{}{}
		s.mu.Unlock()
		success++
	}

	s.mu.Lock()
	j.TotalRows = processed
	j.ProcessedRows = processed
	j.SuccessCount = success
	j.FailureCount = failure
	now2 := time.Now().UTC()
	j.CompletedAt = &now2
	if failure > 0 && success > 0 {
		j.Status = "partial"
	} else if failure > 0 && success == 0 {
		j.Status = "failed"
	} else {
		j.Status = "completed"
	}
	s.mu.Unlock()
	return nil
}

func (s *InMemoryImportService) recordError(jobID string, rowNum int, raw string, msg string) {
	e := model.ImportError{ID: uuid.New().String(), ImportJobID: jobID, RowNumber: rowNum, RawData: raw, ErrorMessage: msg}
	s.mu.Lock()
	s.errors[jobID] = append(s.errors[jobID], e)
	s.mu.Unlock()
}

func (s *InMemoryImportService) RetryFailed(originalJobID, newJobID string) error {
	s.mu.RLock()
	errs := s.errors[originalJobID]
	s.mu.RUnlock()
	if len(errs) == 0 {
		return nil
	}

	// create new job record
	now := time.Now().UTC()
	j := &model.ImportJob{ID: newJobID, FileName: "retry-" + originalJobID, Status: "processing", CreatedAt: now, StartedAt: &now}
	s.mu.Lock()
	s.jobs[newJobID] = j
	s.mu.Unlock()

	var success, failure int
	for _, e := range errs {
		// parse CSV raw_data
		r := csv.NewReader(strings.NewReader(e.RawData))
		rec, err := r.Read()
		if err != nil {
			s.recordError(newJobID, e.RowNumber, e.RawData, "parse error")
			failure++
			continue
		}
		// reuse validation from ProcessFile logic
		if len(rec) < 5 {
			s.recordError(newJobID, e.RowNumber, e.RawData, "invalid columns")
			failure++
			continue
		}
		code := strings.TrimSpace(rec[0])
		dtype := strings.TrimSpace(rec[1])
		expires := strings.TrimSpace(rec[3])

		if code == "" || (dtype != "percentage" && dtype != "fixed") {
			s.recordError(newJobID, e.RowNumber, e.RawData, "validation failed")
			failure++
			continue
		}
		if _, err := time.Parse(time.RFC3339, expires); err != nil {
			s.recordError(newJobID, e.RowNumber, e.RawData, "invalid expires format")
			failure++
			continue
		}
		// uniqueness
		s.mu.Lock()
		if _, exists := s.codes[code]; exists {
			s.mu.Unlock()
			s.recordError(newJobID, e.RowNumber, e.RawData, "duplicate code")
			failure++
			continue
		}
		s.codes[code] = struct{}{}
		s.mu.Unlock()
		success++
	}

	s.mu.Lock()
	j.TotalRows = len(errs)
	j.ProcessedRows = len(errs)
	j.SuccessCount = success
	j.FailureCount = failure
	now2 := time.Now().UTC()
	j.CompletedAt = &now2
	if failure > 0 && success > 0 {
		j.Status = "partial"
	} else if failure > 0 && success == 0 {
		j.Status = "failed"
	} else {
		j.Status = "completed"
	}
	s.mu.Unlock()
	return nil
}
