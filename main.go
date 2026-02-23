package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// ---------- main ----------

func main() {
	ensureCSVs()

	log.Println("Loading data...")
	reloadData()

	log.Println("Loading user data...")
	initUserData()

	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		for range ticker.C {
			log.Println("Auto-updating CSVs from Google Sheets...")
			updateCSVs()
		}
	}()

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
