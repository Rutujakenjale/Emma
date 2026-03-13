package service

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"coupon-import/internal/metrics"
	"coupon-import/internal/model"

	"github.com/google/uuid"
)

type ImportService struct {
	db *sql.DB
}

// ImportServiceInterface defines the methods required by handlers.
type ImportServiceInterface interface {
	CreateJob(fileName string) (*model.ImportJob, error)
	GetJob(id string) (*model.ImportJob, error)
	ListErrors(jobID string) ([]model.ImportError, error)
	ProcessFile(jobID string, path string) error
	RetryFailed(originalJobID, newJobID string) error
}

func NewImportService(db *sql.DB) *ImportService {
	return &ImportService{db: db}
}

func (s *ImportService) CreateJob(fileName string) (*model.ImportJob, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	start := time.Now()
	_, err := s.db.Exec(`INSERT INTO import_jobs(id, file_name, status, created_at) VALUES (?, ?, 'pending', ?)`, id, fileName, now.Format(time.RFC3339))
	metrics.DBQueryDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.DBErrors.Inc()
		return nil, err
	}
	return &model.ImportJob{ID: id, FileName: fileName, Status: "pending", CreatedAt: now}, nil
}

func (s *ImportService) GetJob(id string) (*model.ImportJob, error) {
	row := s.db.QueryRow(`SELECT id,file_name,status,total_rows,processed_rows,success_count,failure_count,created_at,started_at,completed_at FROM import_jobs WHERE id = ?`, id)
	var j model.ImportJob
	var createdAt, startedAt, completedAt sql.NullString
	if err := row.Scan(&j.ID, &j.FileName, &j.Status, &j.TotalRows, &j.ProcessedRows, &j.SuccessCount, &j.FailureCount, &createdAt, &startedAt, &completedAt); err != nil {
		return nil, err
	}
	if t, err := time.Parse(time.RFC3339, createdAt.String); err == nil {
		j.CreatedAt = t
	}
	if startedAt.Valid {
		if t, err := time.Parse(time.RFC3339, startedAt.String); err == nil {
			j.StartedAt = &t
		}
	}
	if completedAt.Valid {
		if t, err := time.Parse(time.RFC3339, completedAt.String); err == nil {
			j.CompletedAt = &t
		}
	}
	return &j, nil
}

func (s *ImportService) recordError(jobID string, rowNum int, raw string, msg string) error {
	start := time.Now()
	_, err := s.db.Exec(`INSERT INTO import_errors(id, import_job_id, row_number, raw_data, error_message) VALUES(?,?,?,?,?)`, uuid.New().String(), jobID, rowNum, raw, msg)
	metrics.DBQueryDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.DBErrors.Inc()
	}
	return err
}

func (s *ImportService) updateProgress(jobID string, processed, success, failure int) error {
	_, err := s.db.Exec(`UPDATE import_jobs SET processed_rows = processed_rows + ?, success_count = success_count + ?, failure_count = failure_count + ? WHERE id = ?`, processed, success, failure, jobID)
	return err
}

// ProcessFile runs in background and streams the CSV
func (s *ImportService) ProcessFile(jobID string, path string) error {
	// mark started
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE import_jobs SET status='processing', started_at = ? WHERE id = ?`, now, jobID); err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := csv.NewReader(bufio.NewReader(f))
	reader.ReuseRecord = true
	// read header
	header, err := reader.Read()
	if err != nil {
		return err
	}
	_ = header

	batch := make([][]interface{}, 0, 500)
	const batchSize = 500
	rowNum := 1
	var success, failure, processed int

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO promotion_codes(id,code,discount_type,discount_value,expires_at,max_uses,created_at) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}

	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowNum++
		processed++
		if err != nil {
			_ = s.recordError(jobID, rowNum, "", err.Error())
			failure++
			continue
		}
		// expected: code,discount_type,discount_value,expires_at,max_uses
		if len(rec) < 5 {
			_ = s.recordError(jobID, rowNum, strings.Join(rec, ","), "invalid columns")
			failure++
			continue
		}
		code := strings.TrimSpace(rec[0])
		dtype := strings.TrimSpace(rec[1])
		dval, err := strconv.ParseFloat(strings.TrimSpace(rec[2]), 64)
		if err != nil {
			_ = s.recordError(jobID, rowNum, strings.Join(rec, ","), "invalid discount value")
			failure++
			continue
		}
		expires := strings.TrimSpace(rec[3])
		maxUses, err := strconv.Atoi(strings.TrimSpace(rec[4]))
		if err != nil {
			_ = s.recordError(jobID, rowNum, strings.Join(rec, ","), "invalid max_uses")
			failure++
			continue
		}

		if code == "" {
			_ = s.recordError(jobID, rowNum, strings.Join(rec, ","), "code required")
			failure++
			continue
		}
		// basic validation
		if dtype != "percentage" && dtype != "fixed" {
			_ = s.recordError(jobID, rowNum, strings.Join(rec, ","), "invalid discount type")
			failure++
			continue
		}
		// try parse expires
		if _, err := time.Parse(time.RFC3339, expires); err != nil {
			_ = s.recordError(jobID, rowNum, strings.Join(rec, ","), "invalid expires format, use RFC3339")
			failure++
			continue
		}

		// add to batch
		batch = append(batch, []interface{}{uuid.New().String(), code, dtype, dval, expires, maxUses, time.Now().UTC().Format(time.RFC3339)})

		if len(batch) >= batchSize {
			// flush
			if err := s.flushBatch(tx, stmt, batch, jobID, &success, &failure); err != nil {
				tx.Rollback()
				stmt.Close()
				return err
			}
			batch = batch[:0]
		}
		if processed%1000 == 0 {
			_ = s.updateProgress(jobID, 1000, success, failure)
			// commit intermediate
			if err := tx.Commit(); err != nil {
				tx.Rollback()
				stmt.Close()
				return err
			}
			// close previous stmt before creating new one
			if err := stmt.Close(); err != nil {
				return err
			}
			tx, err = s.db.Begin()
			if err != nil {
				return err
			}
			stmt, err = tx.Prepare(`INSERT INTO promotion_codes(id,code,discount_type,discount_value,expires_at,max_uses,created_at) VALUES(?,?,?,?,?,?,?)`)
			if err != nil {
				tx.Rollback()
				return err
			}
		}
	}

	if len(batch) > 0 {
		if err := s.flushBatch(tx, stmt, batch, jobID, &success, &failure); err != nil {
			tx.Rollback()
			stmt.Close()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		stmt.Close()
		return err
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	// final update
	// update totals
	_ = s.updateProgress(jobID, processed, success, failure)
	status := "completed"
	if failure > 0 && success > 0 {
		status = "partial"
	} else if failure > 0 && success == 0 {
		status = "failed"
	}
	completed := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`UPDATE import_jobs SET status = ?, total_rows = ?, completed_at = ? WHERE id = ?`, status, processed, completed, jobID)
	if err != nil {
		return err
	}
	log.Printf("job %s done: processed=%d success=%d failure=%d", jobID, processed, success, failure)
	return nil
}

func (s *ImportService) flushBatch(tx *sql.Tx, stmt *sql.Stmt, batch [][]interface{}, jobID string, success *int, failure *int) error {
	for _, row := range batch {
		_, err := stmt.Exec(row...)
		if err != nil {
			// record error and continue
			if sqliteErr := parseSQLiteError(err); sqliteErr != nil {
				_ = s.recordError(jobID, 0, fmt.Sprint(row), sqliteErr.Error())
			} else {
				_ = s.recordError(jobID, 0, fmt.Sprint(row), err.Error())
			}
			(*failure)++
			continue
		}
		(*success)++
	}
	return nil
}

func parseSQLiteError(err error) error {
	if err == nil {
		return nil
	}
	// naive
	if strings.Contains(err.Error(), "UNIQUE") {
		return errors.New("duplicate code")
	}
	return err
}

func (s *ImportService) ListErrors(jobID string) ([]model.ImportError, error) {
	rows, err := s.db.Query(`SELECT id, import_job_id, row_number, raw_data, error_message FROM import_errors WHERE import_job_id = ? ORDER BY row_number ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ImportError
	for rows.Next() {
		var e model.ImportError
		if err := rows.Scan(&e.ID, &e.ImportJobID, &e.RowNumber, &e.RawData, &e.ErrorMessage); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *ImportService) RetryFailed(originalJobID, newJobID string) error {
	// fetch errors for original job
	rows, err := s.db.Query(`SELECT row_number, raw_data FROM import_errors WHERE import_job_id = ? ORDER BY row_number ASC`, originalJobID)
	if err != nil {
		return err
	}
	defer rows.Close()

	// mark new job as processing
	now := time.Now().UTC()
	if _, err := s.db.Exec(`UPDATE import_jobs SET status='processing', started_at = ? WHERE id = ?`, now.Format(time.RFC3339), newJobID); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO promotion_codes(id,code,discount_type,discount_value,expires_at,max_uses,created_at) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	var success, failure, processed int

	for rows.Next() {
		var rowNum int
		var raw string
		if err := rows.Scan(&rowNum, &raw); err != nil {
			continue
		}
		processed++
		r := csv.NewReader(strings.NewReader(raw))
		rec, err := r.Read()
		if err != nil {
			_ = s.recordError(newJobID, rowNum, raw, "parse error")
			failure++
			continue
		}
		if len(rec) < 5 {
			_ = s.recordError(newJobID, rowNum, raw, "invalid columns")
			failure++
			continue
		}
		code := strings.TrimSpace(rec[0])
		dtype := strings.TrimSpace(rec[1])
		dval, err := strconv.ParseFloat(strings.TrimSpace(rec[2]), 64)
		if err != nil {
			_ = s.recordError(newJobID, rowNum, raw, "invalid discount value")
			failure++
			continue
		}
		expires := strings.TrimSpace(rec[3])
		maxUses, err := strconv.Atoi(strings.TrimSpace(rec[4]))
		if err != nil {
			_ = s.recordError(newJobID, rowNum, raw, "invalid max_uses")
			failure++
			continue
		}

		if code == "" {
			_ = s.recordError(newJobID, rowNum, raw, "code required")
			failure++
			continue
		}
		if dtype != "percentage" && dtype != "fixed" {
			_ = s.recordError(newJobID, rowNum, raw, "invalid discount type")
			failure++
			continue
		}
		if _, err := time.Parse(time.RFC3339, expires); err != nil {
			_ = s.recordError(newJobID, rowNum, raw, "invalid expires format, use RFC3339")
			failure++
			continue
		}

		_, err = stmt.Exec(uuid.New().String(), code, dtype, dval, expires, maxUses, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			if sqliteErr := parseSQLiteError(err); sqliteErr != nil {
				_ = s.recordError(newJobID, rowNum, raw, sqliteErr.Error())
			} else {
				_ = s.recordError(newJobID, rowNum, raw, err.Error())
			}
			failure++
			continue
		}
		success++
	}

	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return err
	}

	// final update for new job
	status := "completed"
	if failure > 0 && success > 0 {
		status = "partial"
	} else if failure > 0 && success == 0 {
		status = "failed"
	}
	completed := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE import_jobs SET status = ?, total_rows = ?, processed_rows = ?, success_count = ?, failure_count = ?, completed_at = ? WHERE id = ?`, status, processed, processed, success, failure, completed, newJobID); err != nil {
		return err
	}
	return nil
}
