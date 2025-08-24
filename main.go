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
	Location    string `json:"location"`
	Division    string `json:"division"`
	Competition string `json:"competition"`
}

type ErrorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail"`
}

func getNextWeekendDates() ([]string, []string) {
	now := time.Now()
	daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}

	nextSaturday := now.AddDate(0, 0, daysUntilSaturday)
	nextSunday := nextSaturday.AddDate(0, 0, 1)

	// Broad date formats based on HTML ("Aug 30, 2025")
	saturdayFormats := []string{
		nextSaturday.Format("Jan 02, 2006"),     // "Aug 30, 2025"
		nextSaturday.Format("Jan 2, 2006"),      // "Aug 30, 2025" (single digit)
		nextSaturday.Format("January 02, 2006"), // "August 30, 2025"
		nextSaturday.Format("01/02/2006"),       // "08/30/2025"
		nextSaturday.Format("Jan. 02, 2006"),    // "Aug. 30, 2025"
	}
	sundayFormats := []string{
		nextSunday.Format("Jan 02, 2006"),
		nextSunday.Format("Jan 2, 2006"),
		nextSunday.Format("January 02, 2006"),
		nextSunday.Format("01/02/2006"),
		nextSunday.Format("Jan. 02, 2006"),
	}

	log.Printf("Looking for weekend date patterns: Saturday %v, Sunday %v", saturdayFormats, sundayFormats)
	return saturdayFormats, sundayFormats
}

func scrapeGotSportSchedule(eventID, clubID string) ([]Game, error) {
	url := fmt.Sprintf("https://system.gotsport.com/org_event/events/%s/schedules?club=%s", eventID, clubID)
	log.Printf("Fetching: %s", url)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return nil, fmt.Errorf("request failed: %v", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HTTP request failed: %v", err)
		return nil, fmt.Errorf("http request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Non-200 status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body: %v", err)
		return nil, fmt.Errorf("read body failed: %v", err)
	}

	html := string(body)
	log.Printf("HTML length: %d chars", len(html))
	log.Printf("HTML snippet: %s...", html[:min(len(html), 1000)])

	games := parseWeekendGames(html, eventID)
	if len(games) == 0 {
		log.Printf("No games found for event %s", eventID)
		return nil, fmt.Errorf("no games found for event %s", eventID)
	}
	return games, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func parseWeekendGames(html, eventID string) []Game {
	var games []Game

	saturdayFormats, sundayFormats := getNextWeekendDates()
	htmlLower := strings.ToLower(html)

	var weekendSections []string

	// Check for date patterns in HTML
	for _, satPattern := range saturdayFormats {
		if strings.Contains(htmlLower, strings.ToLower(satPattern)) {
			section := extractSectionAroundDate(html, satPattern)
			if section != "" {
				weekendSections = append(weekendSections, section)
				log.Printf("Found Saturday section for %s (%d chars)", satPattern, len(section))
			}
		}
	}

	for _, sunPattern := range sundayFormats {
		if strings.Contains(htmlLower, strings.ToLower(sunPattern)) {
			section := extractSectionAroundDate(html, sunPattern)
			if section != "" {
				weekendSections = append(weekendSections, section)
				log.Printf("Found Sunday section for %s (%d chars)", sunPattern, len(section))
			}
		}
	}

	// Fallback: Use full HTML if no date sections found
	if len(weekendSections) == 0 {
		log.Printf("No date sections found, using full HTML as fallback")
		weekendSections = append(weekendSections, html)
	}

	for _, section := range weekendSections {
		sectionGames := findRenoApexGamesInSection(section, html)
		games = append(games, sectionGames...)
	}

	log.Printf("Event %s: Found %d weekend Reno Apex home games", eventID, len(games))
	return games
}

func extractSectionAroundDate(html, dateStr string) string {
	index := strings.Index(strings.ToLower(html), strings.ToLower(dateStr))
	if index == -1 {
		log.Printf("Date pattern %s not found in HTML", dateStr)
		return ""
	}

	start := index - 5000
	if start < 0 {
		start = 0
	}
	end := index + 10000
	if end > len(html) {
		end = len(html)
	}

	return html[start:end]
}

func findRenoApexGamesInSection(section, fullHTML string) []Game {
	var games []Game

	// Relaxed regex to match <tr> rows with 7 <td> cells
	rowPattern := regexp.MustCompile(`(?is)<tr[^>]*>\s*((?:<td[^>]*>.*?</td>\s*){7})</tr>`)
	matches := rowPattern.FindAllStringSubmatch(section, -1)

	log.Printf("Found %d table rows in section", len(matches))

	for i, match := range matches {
		if len(match) >= 2 {
			// Extract <td> cells
			tdPattern := regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
			tdMatches := tdPattern.FindAllStringSubmatch(match[1], -1)
			if len(tdMatches) >= 7 {
				matchID := cleanText(tdMatches[0][1])
				dateTime := cleanText(tdMatches[1][1])
				homeTeam := cleanText(tdMatches[2][1])
				results := cleanText(tdMatches[3][1])
				awayTeam := cleanText(tdMatches[4][1])
				location := cleanText(tdMatches[5][1])
				division := cleanText(tdMatches[6][1])

				log.Printf("Processing match #%s: %s vs %s at %s", matchID, homeTeam, awayTeam, location)

				// Confirm home game by checking for "(H)" in full HTML
				if strings.Contains(strings.ToLower(homeTeam), "reno apex") &&
					results == "-" &&
					isHomeGame(matchID, homeTeam, fullHTML) {
					log.Printf("Confirmed HOME game: %s vs %s (Match #%s)", homeTeam, awayTeam, matchID)

					game := Game{
						HomeTeam:    homeTeam,
						AwayTeam:    awayTeam,
						Location:    location,
						Division:    division,
						Competition: division,
					}

					// Parse date and time
					parsedDate, parsedTime := parseDateTime(dateTime)
					game.Date = parsedDate
					game.Time = parsedTime

					if game.Date != "" && game.Time != "TBD" && !isDuplicateGame(games, game) {
						games = append(games, game)
						log.Printf("Added game: %s vs %s at %s %s (%s)", game.HomeTeam, game.AwayTeam, game.Time, game.Location, game.Date)
					} else {
						log.Printf("Skipped game due to invalid date/time or duplicate: %s vs %s (Match #%s)", homeTeam, awayTeam, matchID)
					}
				} else {
					log.Printf("Not a Reno APEX home game or already played: %s vs %s (Match #%s)", homeTeam, awayTeam, matchID)
				}
			} else {
				log.Printf("Incomplete table row %d: found %d <td> elements", i+1, len(tdMatches))
			}
		}
	}

	return games
}

func isHomeGame(matchID, homeTeam, fullHTML string) bool {
	// Check for "(H)" in the secondary section near the match ID
	pattern := regexp.MustCompile(`(?is)` + regexp.QuoteMeta(matchID) + `.*?` + regexp.QuoteMeta(homeTeam) + `\s*\(H\)`)
	return pattern.MatchString(fullHTML)
}

func cleanText(text string) string {
	re := regexp.MustCompile(`(?s)<.*?>`)
	cleaned := re.ReplaceAllString(text, "")
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.Trim(cleaned, ".,;:-")
	return cleaned
}

func parseDateTime(dateTime string) (string, string) {
	// Pattern for "Aug 30, 2025 1:00PM PDT"
	re := regexp.MustCompile(`(?i)([A-Za-z]+\.? \d{1,2}, \d{4})\s+([\d:]+[AP]M [A-Za-z]+)`)
	matches := re.FindStringSubmatch(dateTime)
	if len(matches) >= 3 {
		dateStr := matches[1]
		timeStr := matches[2]

		parsedDate, err := time.Parse("Jan 02, 2006", dateStr)
		if err != nil {
			parsedDate, err = time.Parse("January 02, 2006", dateStr)
		}
		if err != nil {
			parsedDate, err = time.Parse("Jan. 02, 2006", dateStr)
		}
		if err == nil {
			log.Printf("Parsed date: %s, time: %s", parsedDate.Format("2006-01-02"), timeStr)
			return parsedDate.Format("2006-01-02"), timeStr
		}
	}

	// Fallback to next weekend
	log.Printf("Failed to parse date/time: %s", dateTime)
	now := time.Now()
	daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}
	nextSaturday := now.AddDate(0, 0, daysUntilSaturday)
	return nextSaturday.Format("2006-01-02"), "TBD"
}

func isDuplicateGame(existingGames []Game, newGame Game) bool {
	for _, existing := range existingGames {
		if existing.Date == newGame.Date &&
			existing.Time == newGame.Time &&
			strings.EqualFold(existing.HomeTeam, newGame.HomeTeam) &&
			strings.EqualFold(existing.AwayTeam, newGame.AwayTeam) {
			return true
		}
	}
	return false
}

func scrapeECNLSchedule() ([]Game, error) {
	return []Game{}, nil
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	eventID := r.URL.Query().Get("eventid")
	clubID := r.URL.Query().Get("clubid")

	if eventID == "" || clubID == "" {
		log.Printf("Missing parameters: eventid=%s, clubid=%s", eventID, clubID)
		errResp := ErrorResponse{Error: "Missing parameters", Detail: "eventid and clubid are required"}
		http.Error(w, toJSON(errResp), http.StatusBadRequest)
		return
	}

	log.Printf("Processing request: eventid=%s, clubid=%s", eventID, clubID)

	var games []Game
	var err error

	if eventID == "ecnl" {
		games, err = scrapeECNLSchedule()
	} else {
		games, err = scrapeGotSportSchedule(eventID, clubID)
	}

	if err != nil {
		log.Printf("Scrape failed: %v", err)
		errResp := ErrorResponse{Error: "Scrape failed", Detail: err.Error()}
		http.Error(w, toJSON(errResp), http.StatusInternalServerError)
		return
	}

	log.Printf("Returning %d games", len(games))
	json.NewEncoder(w).Encode(games)
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error": "JSON encoding failed"}`
	}
	return string(b)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{
		"status":      "healthy",
		"service":     "Fixed Home/Away GotSport Parser",
		"version":     "12.5",
		"timestamp":   time.Now().Format(time.RFC3339),
		"description": "Enhanced table-based parsing with (H) check and error reporting",
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
		response := "Fixed Home/Away GotSport Parser v12.5\n\n" +
			"Enhanced table-based parsing with (H) check and error reporting!\n\n" +
			"Endpoints:\n" +
			"- /health\n" +
			"- /schedule?eventid=44145&clubid=12893"
		fmt.Fprintf(w, response)
	})

	log.Printf("Fixed Home/Away GotSport Parser v12.5 starting on port %s", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server start failed: %v", err)
	}
}
