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

func getNextWeekendDates() (string, string) {
	now := time.Now()
	
	// Calculate days until next Saturday
	daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7 // If today is Saturday, get next Saturday
	}
	
	nextSaturday := now.AddDate(0, 0, daysUntilSaturday)
	nextSunday := nextSaturday.AddDate(0, 0, 1)
	
	// Format as they appear in HTML: "Aug 30, 2025"
	saturdayStr := nextSaturday.Format("Jan 02, 2006")
	sundayStr := nextSunday.Format("Jan 02, 2006")
	
	log.Printf("Looking for weekend dates: %s and %s", saturdayStr, sundayStr)
	
	return saturdayStr, sundayStr
}

func scrapeGotSportSchedule(eventID, clubID string) ([]Game, error) {
	url := fmt.Sprintf("https://system.gotsport.com/org_event/events/%s/schedules?club=%s", eventID, clubID)
	
	log.Printf("Fetching: %s", url)
	
	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	html := string(body)
	log.Printf("HTML length: %d chars", len(html))
	
	// Only parse weekend games
	games := parseWeekendGames(html, eventID)
	
	log.Printf("Found %d weekend games", len(games))
	return games, nil
}

func parseWeekendGames(html, eventID string) []Game {
	var games []Game
	
	// Get next weekend dates
	saturdayStr, sundayStr := getNextWeekendDates()
	
	// Only look for these specific dates in the HTML
	htmlLower := strings.ToLower(html)
	saturdayLower := strings.ToLower(saturdayStr)
	sundayLower := strings.ToLower(sundayStr)
	
	log.Printf("Searching for: '%s' and '%s'", saturdayLower, sundayLower)
	
	// Find weekend date mentions
	weekendSections := []string{}
	
	if strings.Contains(htmlLower, saturdayLower) {
		section := extractSectionAroundDate(html, saturdayStr)
		if section != "" {
			weekendSections = append(weekendSections, section)
			log.Printf("Found Saturday section (%d chars)", len(section))
		}
	}
	
	if strings.Contains(htmlLower, sundayLower) {
		section := extractSectionAroundDate(html, sundayStr)
		if section != "" {
			weekendSections = append(weekendSections, section)
			log.Printf("Found Sunday section (%d chars)", len(section))
		}
	}
	
	// Parse Reno Apex games from weekend sections only
	for _, section := range weekendSections {
		sectionGames := findRenoApexGamesInSection(section)
		games = append(games, sectionGames...)
	}
	
	log.Printf("Event %s: Found %d weekend Reno Apex home games", eventID, len(games))
	return games
}

func extractSectionAroundDate(html, dateStr string) string {
	// Find the date in HTML and extract surrounding context
	index := strings.Index(strings.ToLower(html), strings.ToLower(dateStr))
	if index == -1 {
		return ""
	}
	
	// Get section around the date (enough to capture the day's games)
	start := index - 1000
	if start < 0 {
		start = 0
	}
	end := index + 3000 // Enough for several games
	if end > len(html) {
		end = len(html)
	}
	
	return html[start:end]
}

func findRenoApexGamesInSection(section string) []Game {
	var games []Game
	
	// Find all Reno Apex mentions in this section
	sectionLower := strings.ToLower(section)
	renoIndices := findAllOccurrences(sectionLower, "reno apex")
	
	for _, index := range renoIndices {
		game := extractGameFromContext(section, index)
		// Only include HOME games
		if game.HomeTeam != "" && strings.Contains(strings.ToLower(game.HomeTeam), "reno apex") {
			games = append(games, game)
			log.Printf("Weekend home game: %s vs %s", game.HomeTeam, game.AwayTeam)
		}
	}
	
	return games
}

func findAllOccurrences(text, substr string) []int {
	var indices []int
	start := 0
	for {
		index := strings.Index(text[start:], substr)
		if index == -1 {
			break
		}
		indices = append(indices, start+index)
		start = start + index + len(substr)
	}
	return indices
}

func extractGameFromContext(section string, index int) Game {
	// Get context around this specific Reno Apex mention
	start := index - 200
	if start < 0 {
		start = 0
	}
	end := index + 500
	if end > len(section) {
		end = len(section)
	}
	
	context := section[start:end]
	
	game := Game{}
	
	// Extract game details from this small context
	game.HomeTeam = extractTeamName(context, "reno apex")
	game.AwayTeam = findOpponentInContext(context)
	game.Date = findDateInContext(context)
	game.Time = findTimeInContext(context)
	game.Venue, game.Field = findVenueAndFieldInContext(context)
	game.Division = findDivisionInContext(context)
	game.Competition = game.Division
	
	return game
}

func extractTeamName(context, hint string) string {
	lines := strings.Split(context, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), hint) {
			// Look for team name in <a> tag
			if start := strings.Index(line, ">"); start != -1 {
				if end := strings.Index(line[start+1:], "<"); end != -1 {
					teamName := strings.TrimSpace(line[start+1 : start+1+end])
					if len(teamName) > 10 && strings.Contains(strings.ToLower(teamName), hint) {
						return teamName
					}
				}
			}
		}
	}
	return ""
}

func findOpponentInContext(context string) string {
	lines := strings.Split(context, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "<a href=") && !strings.Contains(strings.ToLower(line), "reno") && !strings.Contains(line, "schedules?pitch") {
			// Extract team name from <a> tag
			if start := strings.Index(line, ">"); start != -1 {
				if end := strings.Index(line[start+1:], "<"); end != -1 {
					opponent := strings.TrimSpace(line[start+1 : start+1+end])
					if len(opponent) > 10 && strings.Contains(opponent, " ") {
						return opponent
					}
				}
			}
		}
	}
	return "TBD"
}

func findDateInContext(context string) string {
	// Convert weekend date back to YYYY-MM-DD format
	now := time.Now()
	daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}
	
	nextSaturday := now.AddDate(0, 0, daysUntilSaturday)
	nextSunday := nextSaturday.AddDate(0, 0, 1)
	
	saturdayFormatted := nextSaturday.Format("Jan 02, 2006")
	sundayFormatted := nextSunday.Format("Jan 02, 2006")
	
	if strings.Contains(context, saturdayFormatted) {
		return nextSaturday.Format("2006-01-02")
	}
	if strings.Contains(context, sundayFormatted) {
		return nextSunday.Format("2006-01-02")
	}
	
	return nextSaturday.Format("2006-01-02") // Default to Saturday
}

func findTimeInContext(context string) string {
	// Look for time patterns like "10:30 AM PDT"
	words := strings.Fields(context)
	for i, word := range words {
		if strings.Contains(word, ":") && i+1 < len(words) {
			next := words[i+1]
			if strings.HasPrefix(next, "AM") || strings.HasPrefix(next, "PM") {
				return word + " " + strings.Fields(next)[0]
			}
		}
	}
	return "TBD"
}

func findVenueAndFieldInContext(context string) (string, string) {
	lines := strings.Split(context, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if (strings.Contains(line, "Complex") || strings.Contains(line, "Park")) && strings.Contains(line, "<a href=") {
			// Extract location from <a> tag
			if start := strings.Index(line, ">"); start != -1 {
				if end := strings.Index(line[start+1:], "<"); end != -1 {
					location := strings.TrimSpace(line[start+1 : start+1+end])
					if strings.Contains(location, " - ") {
						parts := strings.SplitN(location, " - ", 2)
						return parts[0], parts[1]
					}
					return location, ""
				}
			}
		}
	}
	return "TBD", ""
}

func findDivisionInContext(context string) string {
	if strings.Contains(context, "Premier") {
		return "Premier"
	}
	if strings.Contains(context, "Gold") {
		return "Gold"
	}
	return "League"
}

func scrapeECNLSchedule() ([]Game, error) {
	// Simplified ECNL - return empty for now
	return []Game{}, nil
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

	log.Printf("Weekend-only request: %s/%s", eventID, clubID)

	var games []Game
	var err error
	
	if eventID == "ecnl" {
		games, err = scrapeECNLSchedule()
	} else {
		games, err = scrapeGotSportSchedule(eventID, clubID)
	}
	
	if err != nil {
		log.Printf("Error: %v", err)
		games = []Game{}
	}

	log.Printf("Returning %d weekend games", len(games))
	json.NewEncoder(w).Encode(games)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "healthy",
		"service":     "Weekend-Only GotSport Parser",
		"version":     "10.0-weekend-only",
		"timestamp":   time.Now().Format(time.RFC3339),
		"description": "Only processes next weekend games - much faster",
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/schedule", scheduleHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Weekend-Only GotSport Parser v10.0\n\nOnly looks for next Saturday/Sunday games - ultra fast!\n\nEndpoints:\n- /health\n- /schedule?eventid=44145&clubid=12893")
	})

	log.Printf("Weekend-Only GotSport Parser v10.0 starting on port %s", port)
	log.Printf("Will only process next weekend games for maximum speed")
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server start failed: %v", err)
	}
}
