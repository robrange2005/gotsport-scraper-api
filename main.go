package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
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
	
	log.Printf("Fetching from: %s", url)
	
	// Shorter timeout to prevent hanging
	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []Game{}, fmt.Errorf("failed to create request: %v", err)
	}
	
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HTTP request failed: %v", err)
		return []Game{}, nil // Return empty instead of error
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		log.Printf("HTTP %d from GotSport", resp.StatusCode)
		return []Game{}, nil
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response: %v", err)
		return []Game{}, nil
	}
	
	html := string(body)
	log.Printf("Retrieved %d chars from event %s", len(html), eventID)
	
	// Quick parsing with timeout protection
	games := parseGotSportHTMLFast(html, eventID)
	log.Printf("Found %d Reno Apex home games", len(games))
	
	return games, nil
}

func parseGotSportHTMLFast(html, eventID string) []Game {
	var games []Game
	
	// Set parsing timeout
	done := make(chan []Game, 1)
	
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Parsing panic recovered: %v", r)
				done <- []Game{}
			}
		}()
		
		result := parseWithSimpleRegex(html)
		done <- result
	}()
	
	// Wait for parsing or timeout
	select {
	case result := <-done:
		return result
	case <-time.After(10 * time.Second):
		log.Printf("Parsing timeout - returning empty")
		return []Game{}
	}
}

func parseWithSimpleRegex(html string) []Game {
	var games []Game
	
	// Simple approach: find all Reno Apex home games using basic patterns
	lines := strings.Split(html, "\n")
	
	var currentGame Game
	inRenoRow := false
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Check if this is a Reno Apex home team row
		if strings.Contains(line, "Reno APEX Soccer Club") && 
		   strings.Contains(line, "<td class=") {
			
			// Extract team name
			if teamName := extractTeamNameSimple(line); teamName != "" {
				currentGame.HomeTeam = teamName
				inRenoRow = true
			}
		}
		
		// If we're processing a Reno row, look for other data
		if inRenoRow {
			
			// Look for date/time
			if strings.Contains(line, "AM ") || strings.Contains(line, "PM ") {
				if timeStr := extractTimeSimple(line); timeStr != "" {
					currentGame.Time = timeStr
				}
				if dateStr := extractDateSimple(line); dateStr != "" {
					currentGame.Date = dateStr
				}
			}
			
			// Look for opponent (away team)
			if strings.Contains(line, "</a>") && !strings.Contains(line, "Reno APEX") {
				if opponentName := extractTeamNameSimple(line); opponentName != "" {
					currentGame.AwayTeam = opponentName
				}
			}
			
			// Look for venue
			if strings.Contains(line, "schedules?pitch=") {
				if venue := extractVenueSimple(line); venue != "" {
					parts := strings.Split(venue, " - ")
					if len(parts) >= 2 {
						currentGame.Venue = parts[0]
						currentGame.Field = parts[1]
					} else {
						currentGame.Venue = venue
					}
				}
			}
			
			// Look for division
			if strings.Contains(line, "schedules?group=") {
				if division := extractDivisionSimple(line); division != "" {
					currentGame.Division = division
					currentGame.Competition = division
				}
			}
			
			// End of row - check if we have a complete game
			if strings.Contains(line, "</tr>") {
				if currentGame.HomeTeam != "" && currentGame.AwayTeam != "" {
					games = append(games, currentGame)
					log.Printf("Parsed: %s vs %s on %s at %s", 
						currentGame.HomeTeam, currentGame.AwayTeam, 
						currentGame.Date, currentGame.Time)
				}
				currentGame = Game{}
				inRenoRow = false
			}
		}
	}
	
	return games
}

func extractTeamNameSimple(line string) string {
	re := regexp.MustCompile(`<a[^>]*>([^<]+)</a>`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func extractTimeSimple(line string) string {
	re := regexp.MustCompile(`(\d{1,2}:\d{2} [AP]M)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractDateSimple(line string) string {
	re := regexp.MustCompile(`([A-Z][a-z]{2} \d{1,2}, \d{4})`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return convertDate(matches[1])
	}
	return ""
}

func extractVenueSimple(line string) string {
	re := regexp.MustCompile(`<a[^>]*>([^<]+)</a>`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func extractDivisionSimple(line string) string {
	re := regexp.MustCompile(`<a[^>]*>([^<]+)</a>`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func convertDate(dateStr string) string {
	monthMap := map[string]string{
		"Jan": "01", "Feb": "02", "Mar": "03", "Apr": "04",
		"May": "05", "Jun": "06", "Jul": "07", "Aug": "08",
		"Sep": "09", "Oct": "10", "Nov": "11", "Dec": "12",
	}
	
	parts := strings.Fields(dateStr)
	if len(parts) >= 3 {
		month := monthMap[parts[0]]
		day := strings.TrimSuffix(parts[1], ",")
		year := parts[2]
		
		if len(day) == 1 {
			day = "0" + day
		}
		
		if month != "" {
			return fmt.Sprintf("%s-%s-%s", year, month, day)
		}
	}
	return dateStr
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	eventID := r.URL.Query().Get("eventid")
	clubID := r.URL.Query().Get("clubid")
	
	if eventID == "" || clubID == "" {
		http.Error(w, `{"error": "Missing parameters"}`, 400)
		return
	}

	log.Printf("Request: EventID=%s, ClubID=%s", eventID, clubID)

	var games []Game
	
	if eventID == "ecnl" {
		// ECNL placeholder for now
		games = []Game{}
	} else {
		games, _ = scrapeGotSportSchedule(eventID, clubID)
	}

	log.Printf("Returning %d games", len(games))
	json.NewEncoder(w).Encode(games)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"status":    "healthy",
		"service":   "Timeout-Safe Parser",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "9.0-timeout-safe",
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
		fmt.Fprintf(w, "Timeout-Safe Parser v9.0\n\nFast parsing with timeout protection to prevent hanging.")
	})

	log.Printf("Timeout-Safe Parser v9.0 starting on port %s", port)
	log.Printf("Will respond quickly or return empty array")
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
