package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func upload(wg *sync.WaitGroup, id int, filePath string) {
	defer wg.Done()
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("%d: open error: %v\n", id, err)
		return
	}
	defer file.Close()

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	part, err := mw.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		fmt.Printf("%d: mw create error: %v\n", id, err)
		return
	}
	if _, err := io.Copy(part, file); err != nil {
		fmt.Printf("%d: copy error: %v\n", id, err)
		return
	}
	mw.Close()

	req, err := http.NewRequest("POST", "http://localhost:9090/api/v1/imports", &b)
	if err != nil {
		fmt.Printf("%d: newreq error: %v\n", id, err)
		return
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	dur := time.Since(start)
	if err != nil {
		fmt.Printf("%d: request error: %v (%.3fs)\n", id, err, dur.Seconds())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("%d: %d %s (%.3fs)\n", id, resp.StatusCode, string(bytes.TrimSpace(body)), dur.Seconds())
}

func main() {
	fmt.Println("starting load test")
	filePath := "testdata/good_sample.csv"
	var wg sync.WaitGroup
	n := 5
	wg.Add(n)
	for i := 0; i < n; i++ {
		go upload(&wg, i+1, filePath)
	}
	wg.Wait()
}
