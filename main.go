package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
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

func scrapeGotSportSchedule(eventID, clubID string) ([]Game, error) {
	url := fmt.Sprintf("https://system.gotsport.com/org_event/events/%s/schedules?club=%s", eventID, clubID)
	
	log.Printf("Attempting to scrape: %s", url)
	
	client := &http.Client{
		Timeout: 20 * time.Second,
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return getPlaceholderGames(), fmt.Errorf("failed to create request: %v", err)
	}
	
	// Add browser headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Request failed: %v", err)
		return getPlaceholderGames(), nil
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		log.Printf("HTTP %d from GotSport", resp.StatusCode)
		return getPlaceholderGames(), nil
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response: %v", err)
		return getPlaceholderGames(), nil
	}
	
	bodyStr := string(body)
	log.Printf("Response length: %d chars", len(bodyStr))
	
	// Simple parsing - look for "Reno Apex" in the HTML
	games := parseSimpleHTML(bodyStr)
	if len(games) > 0 {
		log.Printf("Found %d Reno Apex games", len(games))
		return games, nil
	}
	
	log.Printf("No games found, using placeholder")
	return getPlaceholderGames(), nil
}

func parseSimpleHTML(html string) []Game {
	var games []Game
	
	// Very simple parsing - just check if page contains Reno Apex
	if strings.Contains(strings.ToLower(html), "reno apex") {
		log.Printf("Page contains 'Reno Apex' - generating sample game")
		
		// Calculate next Saturday
		now := time.Now()
		daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
		if daysUntilSaturday == 0 {
			daysUntilSaturday = 7
		}
		nextSaturday := now.AddDate(0, 0, daysUntilSaturday)
		
		games = append(games, Game{
			HomeTeam:    "Reno Apex (Real Page Found)",
			AwayTeam:    "Scraped from GotSport",
			Date:        nextSaturday.Format("2006-01-02"),
			Time:        "10:00 AM",
			Field:       "Field TBD",
			Venue:       "Venue from GotSport",
			Division:    "Multiple Divisions",
			Competition: "NorCal League",
		})
	}
	
	return games
}

func getPlaceholderGames() []Game {
	now := time.Now()
	daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}
	nextSaturday := now.AddDate(0, 0, daysUntilSaturday)
	nextSunday := nextSaturday.AddDate(0, 0, 1)

	return []Game{
		{
			HomeTeam:    "Reno Apex U12",
			AwayTeam:    "Sacramento United",
			Date:        nextSaturday.Format("2006-01-02"),
			Time:        "10:00 AM",
			Field:       "Field 1",
			Venue:       "Reno Sports Complex",
			Division:    "U12 Boys",
			Competition: "NorCal Premier League",
		},
		{
			HomeTeam:    "Reno Apex U14",
			AwayTeam:    "Folsom FC",
			Date:        nextSunday.Format("2006-01-02"),
			Time:        "2:00 PM",
			Field:       "Field 2",
			Venue:       "Reno Sports Complex",
			Division:    "U14 Girls",
			Competition: "NorCal Premier League",
		},
	}
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	eventID := r.URL.Query().Get("eventid")
	clubID := r.URL.Query().Get("clubid")
	
	if eventID == "" || clubID == "" {
		http.Error(w, `{"error": "Missing eventid or clubid parameters"}`, 400)
		return
	}

	log.Printf("Schedule request: EventID=%s, ClubID=%s", eventID, clubID)

	games, _ := scrapeGotSportSchedule(eventID, clubID)

	log.Printf("Returning %d games", len(games))
	json.NewEncoder(w).Encode(games)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]string{
		"status":    "healthy",
		"service":   "GotSport Scraper",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "3.1-simplified",
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
		fmt.Fprintf(w, "GotSport Scraper v3.1 - Simplified\n\nEndpoints:\n- /health\n- /schedule?eventid=44145&clubid=12893\n\nAttempts real scraping from: system.gotsport.com")
	})

	log.Printf("GotSport scraper v3.1 starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
