package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const csvDir = "data"
const csvDownloadTimeout = 20 * time.Second
const csvDownloadAttempts = 2

func csvPath(name string) string {
	return filepath.Join(csvDir, name)
}

func sheetCSVURL(sheet csvSheet) string {
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv&gid=%s", sheetID, sheet.GID)
}

func sheetGVizCSVURL(sheet csvSheet) string {
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:csv&gid=%s", sheetID, sheet.GID)
}

func downloadCSV(url string) ([]byte, error) {
	client := &http.Client{Timeout: csvDownloadTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "ffrk-explorer-csv-updater/1.0")
	req.Header.Set("Accept", "text/csv,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func downloadCSVWithRetry(url string, attempts int) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		data, err := downloadCSV(url)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if attempt < attempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return nil, fmt.Errorf("download %s failed after %d attempt(s): %w", url, attempts, lastErr)
}

func downloadSheetCSV(sheet csvSheet) ([]byte, error) {
	urls := []string{sheetCSVURL(sheet), sheetGVizCSVURL(sheet)}
	var errs []string
	for _, url := range urls {
		data, err := downloadCSVWithRetry(url, csvDownloadAttempts)
		if err == nil {
			return data, nil
		}
		errs = append(errs, err.Error())
	}
	return nil, fmt.Errorf("all CSV endpoints failed for gid=%s: %s", sheet.GID, strings.Join(errs, " | "))
}

func validateCSV(data []byte, expectedHeaders []string, currentRowCount int) error {
	r := csv.NewReader(bytes.NewReader(data))
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("parse CSV: %w", err)
	}
	if len(records) < 1 {
		return fmt.Errorf("CSV has no rows")
	}
	header := records[0]
	headerSet := make(map[string]bool, len(header))
	for _, h := range header {
		headerSet[h] = true
	}
	for _, exp := range expectedHeaders {
		if !headerSet[exp] {
			return fmt.Errorf("missing expected header column %q", exp)
		}
	}
	newRowCount := len(records) - 1 // exclude header
	if currentRowCount > 0 && newRowCount < currentRowCount/2 {
		return fmt.Errorf("row count %d is less than 50%% of current %d", newRowCount, currentRowCount)
	}
	fieldCount := len(header)
	for i, rec := range records[1:] {
		if len(rec) > fieldCount*2 {
			return fmt.Errorf("row %d has %d fields (header has %d), possible mangled row", i+1, len(rec), fieldCount)
		}
	}
	return nil
}

func countCSVRows(filename string) int {
	f, err := os.Open(csvPath(filename))
	if err != nil {
		return 0
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return 0
	}
	if len(records) < 1 {
		return 0
	}
	return len(records) - 1
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func buildStagedAppData(updates map[string][]byte) (*AppData, error) {
	stageDir, err := os.MkdirTemp(csvDir, ".csv-stage-*")
	if err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stageDir)

	entries, err := os.ReadDir(csvDir)
	if err != nil {
		return nil, fmt.Errorf("read csv dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".csv") {
			continue
		}
		src := csvPath(entry.Name())
		dst := filepath.Join(stageDir, entry.Name())
		if err := copyFile(src, dst); err != nil {
			return nil, fmt.Errorf("copy %s to stage: %w", entry.Name(), err)
		}
	}
	for name, data := range updates {
		if err := os.WriteFile(filepath.Join(stageDir, name), data, 0o644); err != nil {
			return nil, fmt.Errorf("write staged %s: %w", name, err)
		}
	}

	next, err := buildAppDataFromCSVDir(stageDir)
	if err != nil {
		return nil, fmt.Errorf("staged data build failed: %w", err)
	}
	return next, nil
}

func ensureCSVs() {
	if err := os.MkdirAll(csvDir, 0o755); err != nil {
		log.Fatalf("Failed to create data directory %s: %v", csvDir, err)
	}
	for _, sheet := range csvSheets {
		path := csvPath(sheet.Filename)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		log.Printf("Missing %s, downloading...", sheet.Filename)
		data, err := downloadSheetCSV(sheet)
		if err != nil {
			log.Fatalf("Failed to download %s: %v", sheet.Filename, err)
		}
		if err := validateCSV(data, sheet.ExpectedHeaders, 0); err != nil {
			log.Fatalf("Validation failed for %s: %v", sheet.Filename, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			log.Fatalf("Failed to write %s: %v", path, err)
		}
		log.Printf("Downloaded %s (%d bytes)", sheet.Filename, len(data))
	}
}

func updateCSVs() {
	updates := make(map[string][]byte)
	for _, sheet := range csvSheets {
		data, err := downloadSheetCSV(sheet)
		if err != nil {
			log.Printf("WARNING: failed to download %s: %v", sheet.Filename, err)
			continue
		}
		currentRows := countCSVRows(sheet.Filename)
		if err := validateCSV(data, sheet.ExpectedHeaders, currentRows); err != nil {
			log.Printf("WARNING: validation failed for %s: %v", sheet.Filename, err)
			continue
		}

		targetFile := csvPath(sheet.Filename)
		currentData, err := os.ReadFile(targetFile)
		if err == nil && bytes.Equal(currentData, data) {
			continue
		}
		if err != nil && !os.IsNotExist(err) {
			log.Printf("WARNING: failed to read %s for change detection: %v", targetFile, err)
			continue
		}

		updates[sheet.Filename] = data
	}

	if len(updates) == 0 {
		return
	}

	log.Printf("Validating staged CSV update set (%d changed files)...", len(updates))
	next, err := buildStagedAppData(updates)
	if err != nil {
		log.Printf("WARNING: staged CSV validation failed; keeping existing CSV files and in-memory data: %v", err)
		return
	}

	for _, sheet := range csvSheets {
		data, ok := updates[sheet.Filename]
		if !ok {
			continue
		}
		targetFile := csvPath(sheet.Filename)
		tmpFile := targetFile + ".tmp"
		if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
			log.Printf("WARNING: failed to write %s: %v", tmpFile, err)
			return
		}
		if err := os.Rename(tmpFile, targetFile); err != nil {
			log.Printf("WARNING: failed to rename %s to %s: %v", tmpFile, targetFile, err)
			return
		}
		log.Printf("Updated %s (%d bytes)", sheet.Filename, len(data))
	}

	dataLock.Lock()
	appData = next
	dataLock.Unlock()
	log.Printf("Reloaded %d characters", len(next.Characters))
	log.Println("Data reload complete.")
}
