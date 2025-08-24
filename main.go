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
	
	log.Printf("Fetching real GotSport data from: %s", url)
	
	client := &http.Client{
		Timeout: 25 * time.Second,
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	
	// Add headers to look like a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
	
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()
	
	log.Printf("GotSport response: %d %s", resp.StatusCode, resp.Status)
	
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}
	
	html := string(body)
	log.Printf("Retrieved %d characters from GotSport event %s", len(html), eventID)
	
	// Parse the actual HTML to find Reno Apex games
	games := parseGotSportHTML(html, eventID)
	log.Printf("Parsed %d Reno Apex games from event %s", len(games), eventID)
	
	return games, nil
}

func parseGotSportHTML(html, eventID string) []Game {
	var games []Game
	
	log.Printf("Parsing GotSport HTML for event %s...", eventID)
	
	// Look for table rows that contain Reno Apex games
	// Based on the HTML structure: <tr class='fz-sm'> contains match data
	rowPattern := regexp.MustCompile(`(?s)<tr class='fz-sm'>(.*?)</tr>`)
	rows := rowPattern.FindAllStringSubmatch(html, -1)
	
	log.Printf("Found %d table rows to examine", len(rows))
	
	for _, row := range rows {
		if len(row) > 1 {
			rowHTML := row[1]
			
			// Check if this row contains a Reno Apex team (either home or away)
			if strings.Contains(strings.ToLower(rowHTML), "reno apex") {
				game := extractGameFromRow(rowHTML)
				if game.HomeTeam != "" && game.AwayTeam != "" {
					// Only include if Reno Apex is the HOME team
					if strings.Contains(strings.ToLower(game.HomeTeam), "reno apex") {
						games = append(games, game)
						log.Printf("Found Reno Apex HOME game: %s vs %s on %s at %s", 
							game.HomeTeam, game.AwayTeam, game.Date, game.Time)
					}
				}
			}
		}
	}
	
	log.Printf("Event %s: Found %d Reno Apex home games", eventID, len(games))
	return games
}

func extractGameFromRow(rowHTML string) Game {
	var game Game
	
	// Extract table cells - GotSport uses <td> elements for each column
	cellPattern := regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)
	cells := cellPattern.FindAllStringSubmatch(rowHTML, -1)
	
	if len(cells) < 6 {
		return game // Not enough data
	}
	
	// Based on GotSport table structure:
	// Column 0: Match #
	// Column 1: Time/Date
	// Column 2: Home Team  
	// Column 3: Results
	// Column 4: Away Team
	// Column 5: Location
	// Column 6: Division
	
	// Extract date and time from column 1
	if len(cells) > 1 {
		timeHTML := cells[1][1]
		game.Date = extractDate(timeHTML)
		game.Time = extractTime(timeHTML)
	}
	
	// Extract home team from column 2
	if len(cells) > 2 {
		homeHTML := cells[2][1]
		game.HomeTeam = extractTeamName(homeHTML)
	}
	
	// Extract away team from column 4  
	if len(cells) > 4 {
		awayHTML := cells[4][1]
		game.AwayTeam = extractTeamName(awayHTML)
	}
	
	// Extract location from column 5
	if len(cells) > 5 {
		locationHTML := cells[5][1]
		venue, field := extractVenueAndField(locationHTML)
		game.Venue = venue
		game.Field = field
	}
	
	// Extract division from column 6
	if len(cells) > 6 {
		divisionHTML := cells[6][1]
		game.Division = extractDivision(divisionHTML)
		game.Competition = game.Division // Use division as competition for now
	}
	
	return game
}

func extractDate(html string) string {
	// Look for date pattern like "Aug 24, 2025"
	datePattern := regexp.MustCompile(`([A-Z][a-z]{2} \d{1,2}, \d{4})`)
	matches := datePattern.FindStringSubmatch(html)
	if len(matches) > 1 {
		// Convert "Aug 24, 2025" to "2025-08-24" format
		return convertDateFormat(matches[1])
	}
	return ""
}

func extractTime(html string) string {
	// Look for time pattern like "1:00 PM PDT"
	timePattern := regexp.MustCompile(`(\d{1,2}:\d{2} [AP]M)`)
	matches := timePattern.FindStringSubmatch(html)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractTeamName(html string) string {
	// Team names are in <a> tags, get the text content
	teamPattern := regexp.MustCompile(`<a[^>]*>([^<]+)</a>`)
	matches := teamPattern.FindStringSubmatch(html)
	if len(matches) > 1 {
		return cleanText(matches[1])
	}
	return ""
}

func extractVenueAndField(html string) (string, string) {
	// Location is in <a> tags within <li> elements
	// Pattern: <li><a href="...">Venue Name - Field Name</a></li>
	locationPattern := regexp.MustCompile(`<a[^>]*>([^<]+)</a>`)
	matches := locationPattern.FindStringSubmatch(html)
	if len(matches) > 1 {
		location := cleanText(matches[1])
		// Split venue and field if separated by " - "
		if strings.Contains(location, " - ") {
			parts := strings.SplitN(location, " - ", 2)
			return parts[0], parts[1]
		}
		return location, ""
	}
	return "", ""
}

func extractDivision(html string) string {
	// Division is in <a> tags
	divisionPattern := regexp.MustCompile(`<a[^>]*>([^<]+)</a>`)
	matches := divisionPattern.FindStringSubmatch(html)
	if len(matches) > 1 {
		return cleanText(matches[1])
	}
	return ""
}

func cleanText(text string) string {
	// Remove HTML tags and clean up text
	text = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(text, "")
	// Remove extra whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func convertDateFormat(dateStr string) string {
	// Convert "Aug 24, 2025" to "2025-08-24"
	monthMap := map[string]string{
		"Jan": "01", "Feb": "02", "Mar": "03", "Apr": "04",
		"May": "05", "Jun": "06", "Jul": "07", "Aug": "08",
		"Sep": "09", "Oct": "10", "Nov": "11", "Dec": "12",
	}
	
	// Parse "Aug 24, 2025"
	parts := strings.Fields(dateStr)
	if len(parts) >= 3 {
		month := monthMap[parts[0]]
		day := strings.TrimSuffix(parts[1], ",")
		year := parts[2]
		
		// Pad day with zero if needed
		if len(day) == 1 {
			day = "0" + day
		}
		
		if month != "" {
			return fmt.Sprintf("%s-%s-%s", year, month, day)
		}
	}
	return dateStr // Return original if parsing fails
}

func scrapeECNLSchedule() ([]Game, error) {
	url := "https://theecnl.com/sports/2023/8/8/ECNLRLG_0808235356.aspx"
	
	log.Printf("Fetching ECNL data from: %s", url)
	
	client := &http.Client{
		Timeout: 25 * time.Second,
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECNL request: %v", err)
	}
	
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", "https://theecnl.com/")
	
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ECNL HTTP request failed: %v", err)
	}
	defer resp.Body.Close()
	
	log.Printf("ECNL response: %d %s", resp.StatusCode, resp.Status)
	
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ECNL HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ECNL response: %v", err)
	}
	
	html := string(body)
	log.Printf("Retrieved %d characters from ECNL", len(html))
	
	// Parse ECNL HTML - this would need to be customized based on ECNL's actual structure
	games := parseECNLHTML(html)
	log.Printf("Parsed %d ECNL games", len(games))
	
	return games, nil
}

func parseECNLHTML(html string) []Game {
	var games []Game
	
	// ECNL parsing would be similar but adapted to their HTML structure
	// For now, return empty since we need to see their actual HTML structure
	log.Printf("ECNL parsing not yet implemented - need to see actual HTML structure")
	
	return games
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	eventID := r.URL.Query().Get("eventid")
	clubID := r.URL.Query().Get("clubid")
	
	if eventID == "" || clubID == "" {
		log.Printf("Missing parameters: eventid=%s, clubid=%s", eventID, clubID)
		http.Error(w, `{"error": "Missing eventid or clubid parameters"}`, 400)
		return
	}

	log.Printf("=== REAL GOTSPORT PARSING ===")
	log.Printf("EventID: %s, ClubID: %s", eventID, clubID)

	var games []Game
	var err error
	
	if eventID == "ecnl" {
		games, err = scrapeECNLSchedule()
	} else {
		games, err = scrapeGotSportSchedule(eventID, clubID)
	}
	
	if err != nil {
		log.Printf("Scraping error for %s: %v", eventID, err)
		games = []Game{} // Return empty array instead of fake data
	}

	log.Printf("=== FINAL RESULT ===")
	log.Printf("EventID %s: Returning %d REAL Reno Apex home games", eventID, len(games))
	
	for i, game := range games {
		log.Printf("Game %d: %s vs %s on %s at %s (%s - %s)", 
			i+1, game.HomeTeam, game.AwayTeam, game.Date, game.Time, game.Venue, game.Field)
	}

	json.NewEncoder(w).Encode(games)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"status":      "healthy",
		"service":     "Real GotSport HTML Parser",
		"timestamp":   time.Now().Format(time.RFC3339),
		"version":     "8.0-real-parsing",
		"description": "Parses actual GotSport HTML structure for real game data",
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
		response := "Real GotSport HTML Parser v8.0\n\n"
		response += "This scraper parses the actual GotSport HTML structure!\n\n"
		response += "Endpoints:\n"
		response += "- GET /health\n"
		response += "- GET /schedule?eventid=44145&clubid=12893\n"
		response += "- GET /schedule?eventid=44142&clubid=12893\n"
		response += "- GET /schedule?eventid=ecnl&clubid=12893\n\n"
		response += "Returns REAL Reno Apex home games or empty array if none found."
		fmt.Fprintf(w, response)
	})

	log.Printf("=== Real GotSport HTML Parser v8.0 ===")
	log.Printf("Starting on port %s", port)
	log.Printf("Now parsing ACTUAL GotSport HTML structure!")
	log.Printf("Will extract real team names, opponents, dates, times, and venues")
	log.Printf("Ready to scrape real data!")
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
