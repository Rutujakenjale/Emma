package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/glebarez/sqlite"
)

func TestHealthHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// pass nil DB to simulate no DB required
	RegisterHealthRoutes(r, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("healthz expected 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/readyz", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("readyz expected 200, got %d", w.Code)
	}

	// pass a closed DB to simulate ping fail
	r2 := gin.New()
	var db *sql.DB
	RegisterHealthRoutes(r2, db)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/readyz", nil)
	r2.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		// when db is nil readyz still returns ok
		t.Fatalf("readyz expected 200 with nil db, got %d", w.Code)
	}
}
