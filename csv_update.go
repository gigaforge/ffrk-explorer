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
	"time"
)

const csvDir = "data"

func csvPath(name string) string {
	return filepath.Join(csvDir, name)
}

func downloadCSV(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
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
		url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv&gid=%s", sheetID, sheet.GID)
		data, err := downloadCSV(url)
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
	anyUpdated := false
	for _, sheet := range csvSheets {
		url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv&gid=%s", sheetID, sheet.GID)
		data, err := downloadCSV(url)
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
		tmpFile := targetFile + ".tmp"
		if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
			log.Printf("WARNING: failed to write %s: %v", tmpFile, err)
			continue
		}
		if err := os.Rename(tmpFile, targetFile); err != nil {
			log.Printf("WARNING: failed to rename %s to %s: %v", tmpFile, targetFile, err)
			continue
		}
		log.Printf("Updated %s (%d bytes)", sheet.Filename, len(data))
		anyUpdated = true
	}
	if anyUpdated {
		log.Println("Reloading data after CSV update...")
		reloadData()
		log.Println("Data reload complete.")
	}
}
