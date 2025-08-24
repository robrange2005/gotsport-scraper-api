package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type Game struct {
	HomeTeam    string `json:"homeTeam"`
	AwayTeam    string `json:"awayTeam"`
	Date        string `json:"date"`
	Time        string `json:"time"`
	Field       string `json:"field"`
	Venue       string `json:"venue"`
	Division    string `json:"division"`
	Competition string `json:"competition"`
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	eventID := r.URL.Query().Get("eventid")
	clubID := r.URL.Query().Get("clubid")
	
	if eventID == "" || clubID == "" {
		http.Error(w, "Missing eventid or clubid parameters", 400)
		return
	}

	log.Printf("Attempting to scrape real data for EventID: %s, ClubID: %s", eventID, clubID)

	// For now, return a message that we're working on real scraping
	// We'll implement the actual scraping in the next step
	games := []Game{
		{
			HomeTeam:    "Reno Apex (Real Data Coming Soon)",
			AwayTeam:    "Check Back Soon",
			Date:        time.Now().AddDate(0, 0, 2).Format("2006-01-02"),
			Time:        "TBD",
			Field:       "Real GotSport Data",
			Venue:       "Coming Soon",
			Division:    "All Divisions",
			Competition: "Live Data",
		},
	}

	log.Printf("Returning placeholder for real scraper - %d games", len(games))
	json.NewEncoder(w).Encode(games)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]string{
		"status":    "healthy - preparing real scraper",
		"timestamp": time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/schedule", scheduleHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "GotSport Real Data Scraper API (In Development)\n\nEndpoints:\n- /health\n- /schedule?eventid=44145&clubid=12893")
	})

	log.Printf("GotSport scraper (development) starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}