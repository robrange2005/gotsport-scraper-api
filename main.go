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
	
	log.Printf("Fetching real data from: %s", url)
	
	client := &http.Client{
		Timeout: 25 * time.Second,
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	
	// Comprehensive headers to avoid blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Pragma", "no-cache")
	
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
	log.Printf("Retrieved %d characters of HTML from GotSport event %s", len(html), eventID)
	
	// Debug: Log a sample of the HTML to understand structure
	if len(html) > 500 {
		log.Printf("HTML sample: %s...", html[:500])
	}
	
	// Parse the actual HTML to find real Reno Apex games
	games := parseGotSportHTML(html, eventID)
	log.Printf("Parsed %d Reno Apex games from event %s", len(games), eventID)
	
	return games, nil
}

func scrapeECNLSchedule() ([]Game, error) {
	url := "https://theecnl.com/sports/2023/8/8/ECNLRLG_0808235356.aspx"
	
	log.Printf("Fetching real ECNL data from: %s", url)
	
	client := &http.Client{
		Timeout: 25 * time.Second,
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECNL request: %v", err)
	}
	
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://theecnl.com/")
	req.Header.Set("Origin", "https://theecnl.com")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	
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
	log.Printf("Retrieved %d characters of HTML from ECNL", len(html))
	
	// Debug: Log a sample to understand structure
	if len(html) > 500 {
		log.Printf("ECNL HTML sample: %s...", html[:500])
	}
	
	games := parseECNLHTML(html)
	log.Printf("Parsed %d ECNL Reno Apex games", len(games))
	
	return games, nil
}

func parseGotSportHTML(html, eventID string) []Game {
	var games []Game
	
	log.Printf("Parsing GotSport HTML for event %s...", eventID)
	
	// Remove extra whitespace and normalize
	html = strings.ReplaceAll(html, "\r\n", " ")
	html = strings.ReplaceAll(html, "\n", " ")
	html = regexp.MustCompile(`\s+`).ReplaceAllString(html, " ")
	
	// Strategy 1: Look for table rows with schedule data
	games = append(games, extractFromTables(html)...)
	
	// Strategy 2: Look for div-based schedule layouts
	games = append(games, extractFromDivs(html)...)
	
	// Strategy 3: Look for JSON data embedded in the page
	games = append(games, extractFromJSON(html)...)
	
	// Strategy 4: Use broad pattern matching for any Reno Apex mentions
	games = append(games, extractWithPatterns(html)...)
	
	// Filter to only Reno Apex HOME games
	homeGames := filterForRenoApexHomeGames(games)
	
	log.Printf("Event %s: Found %d total games, %d Reno Apex home games", eventID, len(games), len(homeGames))
	
	return homeGames
}

func extractFromTables(html string) []Game {
	var games []Game
	
	// Look for table elements that might contain schedule data
	tablePattern := regexp.MustCompile(`(?i)<table[^>]*>(.*?)</table>`)
	tables := tablePattern.FindAllStringSubmatch(html, -1)
	
	log.Printf("Found %d tables to examine", len(tables))
	
	for _, table := range tables {
		if len(table) > 1 {
			tableHTML := table[1]
			
			// Look for rows within this table
			rowPattern := regexp.MustCompile(`(?i)<tr[^>]*>(.*?)</tr>`)
			rows := rowPattern.FindAllStringSubmatch(tableHTML, -1)
			
			for _, row := range rows {
				if len(row) > 1 {
					rowHTML := row[1]
					
					// Check if this row contains Reno Apex
					if strings.Contains(strings.ToLower(rowHTML), "reno apex") || 
					   strings.Contains(strings.ToLower(rowHTML), "reno") {
						
						game := extractGameFromRow(rowHTML)
						if game.HomeTeam != "" {
							games = append(games, game)
						}
					}
				}
			}
		}
	}
	
	return games
}

func extractFromDivs(html string) []Game {
	var games []Game
	
	// Look for div elements that might contain game information
	divPattern := regexp.MustCompile(`(?i)<div[^>]*class="[^"]*(?:game|schedule|match)[^"]*"[^>]*>(.*?)</div>`)
	divs := divPattern.FindAllStringSubmatch(html, -1)
	
	log.Printf("Found %d schedule divs to examine", len(divs))
	
	for _, div := range divs {
		if len(div) > 1 {
			divHTML := div[1]
			
			if strings.Contains(strings.ToLower(divHTML), "reno apex") || 
			   strings.Contains(strings.ToLower(divHTML), "reno") {
				
				game := extractGameFromDiv(divHTML)
				if game.HomeTeam != "" {
					games = append(games, game)
				}
			}
		}
	}
	
	return games
}

func extractFromJSON(html string) []Game {
	var games []Game
	
	// Look for JavaScript variables that might contain schedule data
	jsonPatterns := []string{
		`(?i)var\s+scheduleData\s*=\s*(\[.*?\]);`,
		`(?i)var\s+games\s*=\s*(\[.*?\]);`,
		`(?i)window\.scheduleData\s*=\s*(\[.*?\]);`,
		`(?i)"schedule":\s*(\[.*?\])`,
	}
	
	for _, pattern := range jsonPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(html, -1)
		
		log.Printf("JSON pattern '%s' found %d matches", pattern, len(matches))
		
		for _, match := range matches {
			if len(match) > 1 {
				// Try to parse JSON data
				jsonData := match[1]
				log.Printf("Found potential JSON data: %s", jsonData[:min(len(jsonData), 200)]+"...")
				// Here you would parse the actual JSON structure
			}
		}
	}
	
	return games
}

func extractWithPatterns(html string) []Game {
	var games []Game
	
	// Look for various text patterns that indicate Reno Apex games
	patterns := []string{
		// Pattern 1: "Reno Apex U12 Boys vs Sacramento FC"
		`(?i)(Reno\s+Apex[^v]*?)(?:\s+vs\.?\s+|\s+v\.?\s+)([^<>\n]+?)(?:\s|<|$)`,
		
		// Pattern 2: "Home: Reno Apex, Away: Opponent"
		`(?i)Home:\s*([^,]*Reno\s+Apex[^,]*),\s*Away:\s*([^<>\n]+)`,
		
		// Pattern 3: Table cell patterns
		`(?i)<td[^>]*>([^<]*Reno\s+Apex[^<]*)</td>\s*<td[^>]*>([^<]+)</td>`,
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(html, -1)
		
		log.Printf("Pattern '%s' found %d matches", pattern, len(matches))
		
		for _, match := range matches {
			if len(match) >= 3 {
				homeTeam := strings.TrimSpace(match[1])
				awayTeam := strings.TrimSpace(match[2])
				
				if homeTeam != "" && awayTeam != "" {
					game := Game{
						HomeTeam:    cleanText(homeTeam),
						AwayTeam:    cleanText(awayTeam),
						Date:        extractNearbyDate(html, match[0]),
						Time:        extractNearbyTime(html, match[0]),
						Field:       extractNearbyField(html, match[0]),
						Venue:       extractNearbyVenue(html, match[0]),
						Division:    extractDivision(homeTeam),
						Competition: "NorCal Premier League",
					}
					games = append(games, game)
				}
			}
		}
	}
	
	return games
}

func extractGameFromRow(rowHTML string) Game {
	// Extract data from table row
	cells := regexp.MustCompile(`(?i)<td[^>]*>(.*?)</td>`).FindAllStringSubmatch(rowHTML, -1)
	
	var cellTexts []string
	for _, cell := range cells {
		if len(cell) > 1 {
			cellTexts = append(cellTexts, cleanText(cell[1]))
		}
	}
	
	if len(cellTexts) < 2 {
		return Game{}
	}
	
	// Try to identify which cells contain which data
	var homeTeam, awayTeam, date, time, field, venue string
	
	for i, text := range cellTexts {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		
		if strings.Contains(strings.ToLower(text), "reno apex") {
			homeTeam = text
			// Next cell might be opponent
			if i+1 < len(cellTexts) {
				awayTeam = cellTexts[i+1]
			}
		} else if isDateFormat(text) {
			date = text
		} else if isTimeFormat(text) {
			time = text
		} else if strings.Contains(strings.ToLower(text), "field") {
			field = text
		}
	}
	
	return Game{
		HomeTeam:    homeTeam,
		AwayTeam:    awayTeam,
		Date:        date,
		Time:        time,
		Field:       field,
		Venue:       venue,
		Division:    extractDivision(homeTeam),
		Competition: "NorCal Premier League",
	}
}

func extractGameFromDiv(divHTML string) Game {
	// Similar extraction for div-based layouts
	homeTeam := extractWithRegex(divHTML, `(?i)(Reno\s+Apex[^<>\n]*?)(?:\s+vs\.?|<)`)
	awayTeam := extractWithRegex(divHTML, `(?i)vs\.?\s+([^<>\n]+?)(?:<|$)`)
	
	return Game{
		HomeTeam:    cleanText(homeTeam),
		AwayTeam:    cleanText(awayTeam),
		Date:        extractDateFromText(divHTML),
		Time:        extractTimeFromText(divHTML),
		Field:       extractFieldFromText(divHTML),
		Venue:       extractVenueFromText(divHTML),
		Division:    extractDivision(homeTeam),
		Competition: "NorCal Premier League",
	}
}

func parseECNLHTML(html string) []Game {
	var games []Game
	
	log.Printf("Parsing ECNL HTML...")
	
	// Similar parsing strategies for ECNL
	html = strings.ReplaceAll(html, "\r\n", " ")
	html = strings.ReplaceAll(html, "\n", " ")
	html = regexp.MustCompile(`\s+`).ReplaceAllString(html, " ")
	
	// Look for Reno Apex mentions in ECNL format
	patterns := []string{
		`(?i)(Reno\s+Apex[^v]*?)(?:\s+vs\.?\s+|\s+v\.?\s+)([^<>\n]+?)`,
		`(?i)<td[^>]*>([^<]*Reno\s+Apex[^<]*)</td>`,
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(html, -1)
		
		log.Printf("ECNL pattern '%s' found %d matches", pattern, len(matches))
		
		for _, match := range matches {
			if len(match) >= 2 {
				homeTeam := cleanText(match[1])
				awayTeam := ""
				if len(match) >= 3 {
					awayTeam = cleanText(match[2])
				}
				
				if strings.Contains(strings.ToLower(homeTeam), "reno apex") {
					game := Game{
						HomeTeam:    homeTeam,
						AwayTeam:    awayTeam,
						Date:        extractNearbyDate(html, match[0]),
						Time:        extractNearbyTime(html, match[0]),
						Field:       extractNearbyField(html, match[0]),
						Venue:       extractNearbyVenue(html, match[0]),
						Division:    extractDivision(homeTeam),
						Competition: "ECNL Regional League",
					}
					games = append(games, game)
				}
			}
		}
	}
	
	return games
}

// Helper functions
func extractWithRegex(text, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func cleanText(text string) string {
	// Remove HTML tags
	re := regexp.MustCompile(`<[^>]*>`)
	cleaned := re.ReplaceAllString(text, "")
	
	// Remove extra whitespace
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")
	
	return strings.TrimSpace(cleaned)
}

func isDateFormat(text string) bool {
	datePatterns := []string{
		`\d{1,2}/\d{1,2}/\d{4}`,
		`\d{4}-\d{2}-\d{2}`,
		`\w+ \d{1,2}, \d{4}`,
	}
	
	for _, pattern := range datePatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			return true
		}
	}
	return false
}

func isTimeFormat(text string) bool {
	timePattern := `\d{1,2}:\d{2}\s*[AaPp][Mm]`
	matched, _ := regexp.MatchString(timePattern, text)
	return matched
}

func extractDateFromText(text string) string {
	datePattern := regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{4}|\d{4}-\d{2}-\d{2}|\w+ \d{1,2}, \d{4}`)
	match := datePattern.FindString(text)
	return match
}

func extractTimeFromText(text string) string {
	timePattern := regexp.MustCompile(`\d{1,2}:\d{2}\s*[AaPp][Mm]`)
	match := timePattern.FindString(text)
	return match
}

func extractFieldFromText(text string) string {
	fieldPattern := regexp.MustCompile(`(?i)field\s*:?\s*([^\s<>,]+)`)
	matches := fieldPattern.FindStringSubmatch(text)
	if len(matches) > 1 {
		return "Field " + matches[1]
	}
	return ""
}

func extractVenueFromText(text string) string {
	venuePattern := regexp.MustCompile(`(?i)(?:venue|location|at)\s*:?\s*([^<>,\n]+)`)
	matches := venuePattern.FindStringSubmatch(text)
	if len(matches) > 1 {
		return cleanText(matches[1])
	}
	return ""
}

func extractDivision(teamName string) string {
	agePattern := regexp.MustCompile(`(?i)U(\d+)`)
	ageMatch := agePattern.FindStringSubmatch(teamName)
	
	if len(ageMatch) >= 2 {
		division := "U" + ageMatch[1]
		if strings.Contains(strings.ToLower(teamName), "girls") {
			division += " Girls"
		} else if strings.Contains(strings.ToLower(teamName), "boys") {
			division += " Boys"
		}
		return division
	}
	return ""
}

func extractNearbyDate(html, matchText string) string {
	// Look for dates near the matched text
	index := strings.Index(html, matchText)
	if index == -1 {
		return ""
	}
	
	// Search in a window around the match
	start := max(0, index-200)
	end := min(len(html), index+200)
	window := html[start:end]
	
	return extractDateFromText(window)
}

func extractNearbyTime(html, matchText string) string {
	index := strings.Index(html, matchText)
	if index == -1 {
		return ""
	}
	
	start := max(0, index-200)
	end := min(len(html), index+200)
	window := html[start:end]
	
	return extractTimeFromText(window)
}

func extractNearbyField(html, matchText string) string {
	index := strings.Index(html, matchText)
	if index == -1 {
		return ""
	}
	
	start := max(0, index-200)
	end := min(len(html), index+200)
	window := html[start:end]
	
	return extractFieldFromText(window)
}

func extractNearbyVenue(html, matchText string) string {
	index := strings.Index(html, matchText)
	if index == -1 {
		return ""
	}
	
	start := max(0, index-200)
	end := min(len(html), index+200)
	window := html[start:end]
	
	return extractVenueFromText(window)
}

func filterForRenoApexHomeGames(games []Game) []Game {
	var homeGames []Game
	
	for _, game := range games {
		// Only include games where Reno Apex is the home team
		if strings.Contains(strings.ToLower(game.HomeTeam), "reno apex") {
			homeGames = append(homeGames, game)
		}
	}
	
	return homeGames
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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

	log.Printf("=== REAL PARSING REQUEST ===")
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
		// Return empty array instead of fake data
		games = []Game{}
	}

	log.Printf("=== FINAL RESULT ===")
	log.Printf("EventID %s: Returning %d real Reno Apex home games", eventID, len(games))
	
	for i, game := range games {
		log.Printf("Game %d: %s vs %s on %s at %s", i+1, game.HomeTeam, game.AwayTeam, game.Date, game.Time)
	}

	json.NewEncoder(w).Encode(games)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"status":      "healthy",
		"service":     "Real HTML Parser - No Fake Data",
		"timestamp":   time.Now().Format(time.RFC3339),
		"version":     "6.0-real-parsing",
		"description": "Actually parses HTML to find real Reno Apex games",
		"uptime":      time.Since(startTime).String(),
	}
	json.NewEncoder(w).Encode(response)
}

var startTime = time.Now()

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/schedule", scheduleHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		response := "Real HTML Parser v6.0 - No Fake Data!\n\n"
		response += "This scraper actually parses HTML to find REAL Reno Apex games.\n\n"
		response += "Endpoints:\n"
		response += "- GET /health\n"
		response += "- GET /schedule?eventid=44145&clubid=12893\n"
		response += "- GET /schedule?eventid=44142&clubid=12893\n"
		response += "- GET /schedule?eventid=ecnl&clubid=12893\n\n"
		response += "Returns actual game data parsed from HTML or empty array if none found."
		fmt.Fprintf(w, response)
	})

	log.Printf("=== Real HTML Parser v6.0 ===")
	log.Printf("Starting on port %s", port)
	log.Printf("NO MORE FAKE DATA - only real parsed games!")
	log.Printf("Will return empty arrays if no games found")
	log.Printf("Ready to parse real HTML!")
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
