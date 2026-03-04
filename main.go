package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

const csvUpdateInterval = 6 * time.Hour

// ---------- main ----------

func main() {
	ensureCSVs()

	log.Println("Loading data...")
	if err := reloadData(); err != nil {
		log.Fatalf("Initial data load failed: %v", err)
	}

	log.Println("Loading user data...")
	initUserData()

	go runCSVAutoUpdateLoop()

	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir("images"))))
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/char/", charHandler)
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/api/characters", characterAPIHandler)
	http.HandleFunc("/api/tiers", tierAPIHandler)
	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/register", registerHandler)
	http.HandleFunc("/api/logout", logoutHandler)
	http.HandleFunc("/api/me", meHandler)
	http.HandleFunc("/api/user/soulbreaks", userSoulbreaksHandler)
	http.HandleFunc("/ffrk_sync.php", syncHandler)

	addr := "0.0.0.0:9090"
	fmt.Printf("Server running at http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func runCSVAutoUpdateLoop() {
	time.Sleep(5 * time.Second)
	log.Println("Attempting immediate CSV update from Google Sheets...")
	updateCSVs()

	ticker := time.NewTicker(csvUpdateInterval)
	defer ticker.Stop()
	for range ticker.C {
		log.Println("Auto-updating CSVs from Google Sheets...")
		updateCSVs()
	}
}
