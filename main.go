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

func getNextWeekendDates() ([]string, []string) {
	now := time.Now()
	daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}

	nextSaturday := now.AddDate(0, 0, daysUntilSaturday)
	nextSunday := nextSaturday.AddDate(0, 0, 1)

	// Support more date formats
	saturdayFormats := []string{
		nextSaturday.Format("January 02, 2006"), // "August 30, 2025"
		nextSaturday.Format("Jan 02, 2006"),     // "Aug 30, 2025"
		nextSaturday.Format("01/02/2006"),       // "08/30/2025"
		nextSaturday.Format("02-Jan-2006"),      // "30-Aug-2025"
		nextSaturday.Format("Jan 02 2006"),      // "Aug 30 2025"
	}
	sundayFormats := []string{
		nextSunday.Format("January 02, 2006"),
		nextSunday.Format("Jan 02, 2006"),
		nextSunday.Format("01/02/2006"),
		nextSunday.Format("02-Jan-2006"),
		nextSunday.Format("Jan 02 2006"),
	}

	log.Printf("Looking for weekend date patterns: Saturday %v, Sunday %v", saturdayFormats, sundayFormats)
	return saturdayFormats, sundayFormats
}

func scrapeGotSportSchedule(eventID, clubID string) ([]Game, error) {
	url := fmt.Sprintf("https://system.gotsport.com/org_event/events/%s/schedules?club=%s", eventID, clubID)
	log.Printf("Fetching: %s", url)

	client := &http.Client{Timeout: 30 * time.Second} // Increased timeout
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return nil, fmt.Errorf("request failed: %v", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
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

	games := parseWeekendGames(html, eventID)
	log.Printf("Found %d weekend games", len(games))
	return games, nil
}

func parseWeekendGames(html, eventID string) []Game {
	var games []Game

	saturdayFormats, sundayFormats := getNextWeekendDates()
	htmlLower := strings.ToLower(html)

	var weekendSections []string

	// Check for Saturday and Sunday date patterns
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

	for _, section := range weekendSections {
		sectionGames := findRenoApexGamesInSection(section)
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

	start := index - 4000 // Increased context
	if start < 0 {
		start = 0
	}
	end := index + 8000
	if end > len(html) {
		end = len(html)
	}

	return html[start:end]
}

func findRenoApexGamesInSection(section string) []Game {
	var games []Game
	lines := strings.Split(section, "\n")

	for i, line := range lines {
		if strings.Contains(line, "- ") && strings.Contains(strings.ToLower(line), "reno apex") {
			parts := strings.Split(line, "- ")
			if len(parts) >= 2 {
				homeTeamPart := strings.TrimSpace(parts[0])
				awayTeamPart := strings.TrimSpace(parts[1])

				if strings.Contains(strings.ToLower(homeTeamPart), "reno apex") {
					log.Printf("Found HOME game: %s - %s", homeTeamPart, awayTeamPart)

					// Capture more context
					start := i - 10
					if start < 0 {
						start = 0
					}
					end := i + 10
					if end > len(lines) {
						end = len(lines)
					}
					context := strings.Join(lines[start:end], "\n")

					game := extractGameFromLine(line, context)
					if game.HomeTeam != "" && !isDuplicateGame(games, game) {
						games = append(games, game)
						log.Printf("Added home game: %s vs %s at %s %s (%s)", 
							game.HomeTeam, game.AwayTeam, game.Time, game.Location, game.Date)
					}
				} else {
					log.Printf("Skipping AWAY game: %s - %s", homeTeamPart, awayTeamPart)
				}
			}
		}
	}

	return games
}

func extractGameFromLine(gameLine string, context string) Game {
	game := Game{}

	parts := strings.Split(gameLine, "- ")
	if len(parts) >= 2 {
		homeRaw := strings.TrimSpace(parts[0])
		game.HomeTeam = extractTeamNameFromText(homeRaw)

		awayRaw := strings.TrimSpace(parts[1])
		game.AwayTeam = extractTeamNameFromText(awayRaw)
	}

	game.Date = findDateInContext(context)
	game.Time = findTimeInContext(context)
	game.Location = findLocationInContext(context)
	game.Division = findDivisionInContext(context)
	game.Competition = game.Division

	return game
}

func extractTeamNameFromText(text string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	cleaned := re.ReplaceAllString(text, "")
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.Trim(cleaned, ".,;:-")
	return cleaned
}

func findDateInContext(context string) string {
	now := time.Now()
	daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}

	nextSaturday := now.AddDate(0, 0, daysUntilSaturday)
	nextSunday := nextSaturday.AddDate(0, 0, 1)

	// Broader date patterns
	datePatterns := []string{
		`January\s+\d{1,2},\s+\d{4}`, // "August 30, 2025"
		`Jan\s+\d{1,2},\s+\d{4}`,    // "Aug 30, 2025"
		`\d{1,2}/\d{1,2}/\d{4}`,     // "08/30/2025"
		`\d{1,2}-[A-Za-z]{3}-\d{4}`, // "30-Aug-2025"
		`Jan\s+\d{1,2}\s+\d{4}`,     // "Aug 30 2025"
	}

	for _, pattern := range datePatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindStringSubmatch(context)
		if len(matches) > 0 {
			dateStr := matches[0]
			parsed, err := time.Parse("January 02, 2006", dateStr)
			if err != nil {
				parsed, err = time.Parse("Jan 02, 2006", dateStr)
			}
			if err != nil {
				parsed, err = time.Parse("01/02/2006", dateStr)
			}
			if err != nil {
				parsed, err = time.Parse("02-Jan-2006", dateStr)
			}
			if err != nil {
				parsed, err = time.Parse("Jan 02 2006", dateStr)
			}
			if err == nil {
				log.Printf("Found date: %s", parsed.Format("2006-01-02"))
				return parsed.Format("2006-01-02")
			}
		}
	}

	// Fallback to Saturday/Sunday
	saturdayFormats, sundayFormats := getNextWeekendDates()
	for _, satPattern := range saturdayFormats {
		if strings.Contains(strings.ToLower(context), strings.ToLower(satPattern)) {
			log.Printf("Date fallback to Saturday: %s", nextSaturday.Format("2006-01-02"))
			return nextSaturday.Format("2006-01-02")
		}
	}
	for _, sunPattern := range sundayFormats {
		if strings.Contains(strings.ToLower(context), strings.ToLower(sunPattern)) {
			log.Printf("Date fallback to Sunday: %s", nextSunday.Format("2006-01-02"))
			return nextSunday.Format("2006-01-02")
		}
	}

	log.Printf("No date found in context")
	return nextSaturday.Format("2006-01-02")
}

func findTimeInContext(context string) string {
	timePatterns := []string{
		`(\d{1,2}:\d{2})\s*(AM|PM)\s*(PDT|PST|PT)?`, // "1:00 PM PDT"
		`(\d{1,2}:\d{2})(?:AM|PM)`,                  // "1:00PM"
		`(\d{1,2}:\d{2}\s*[AP]M)`,                   // "1:00 PM"
	}

	for _, pattern := range timePatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindStringSubmatch(context)
		if len(matches) >= 2 {
			timeStr := strings.TrimSpace(matches[1])
			if len(matches) >= 3 {
				timeStr += " " + strings.ToUpper(strings.TrimSpace(matches[2]))
				if len(matches) >= 4 && matches[3] != "" {
					timeStr += " " + strings.ToUpper(strings.TrimSpace(matches[3]))
				}
			}
			log.Printf("Found time: %s", timeStr)
			return timeStr
		}
	}

	log.Printf("No time found in context")
	return "TBD"
}

func findLocationInContext(context string) string {
	// Broader location patterns
	locationPatterns := []string{
		`<a[^>]*href="[^"]*schedules\?pitch[^"]*"[^>]*>([^<]+)</a>`,
		`([A-Za-z0-9\s\-]+(?:Park|Complex|Field|Stadium|Center|School|High School)[A-Za-z0-9\s\-]*(?:[-–]\s*[A-Za-z0-9\s]+)?)`,
		`\*\s*([^<\n]+(?:Park|Complex|Field|Stadium|Center|School|High School)[^<\n]*(?:[-–]\s*[A-Za-z0-9\s]+)?)`,
		`(Lazy 5 Regional Park|Bishop Manogue High School)[^<\n]*(?:[-–]\s*[A-Za-z0-9\s]+)?`, // Specific venues
	}

	for _, pattern := range locationPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindAllStringSubmatch(context, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				location := strings.TrimSpace(match[1])
				location = strings.Trim(location, "*.,;:")
				if len(location) > 5 && len(location) < 100 {
					log.Printf("Found location: %s", location)
					return location
				}
			}
		}
	}

	// Fallback: Check lines
	lines := strings.Split(context, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Park") || strings.Contains(line, "Field") || 
		   strings.Contains(line, "School") || strings.Contains(line, "Complex") {
			re := regexp.MustCompile(`<[^>]*>`)
			cleaned := re.ReplaceAllString(line, "")
			cleaned = strings.TrimSpace(cleaned)
			if len(cleaned) > 5 && len(cleaned) < 100 {
				log.Printf("Found location (fallback): %s", cleaned)
				return cleaned
			}
		}
	}

	log.Printf("No location found in context")
	return "TBD"
}

func findDivisionInContext(context string) string {
	divisionPatterns := []string{
		`(2010B\s+NPL\s+East|[A-Za-z0-9\s\-/]+NPL[A-Za-z0-9\s\-]*)`,
		`(2007/08\s+North\s+-\s+Yellow|[A-Za-z0-9\s\-/]+)`,
		`(?:Division|League|Group)\s*:?\s*([A-Za-z0-9\s\-/]+)`,
	}

	for _, pattern := range divisionPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindStringSubmatch(context)
		if len(matches) >= 2 {
			division := strings.TrimSpace(matches[1])
			if len(division) > 3 {
				log.Printf("Found division: %s", division)
				return division
			}
		}
	}

	log.Printf("No division found in context")
	return "League"
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
		http.Error(w, `{"error": "Missing parameters"}`, http.StatusBadRequest)
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
		http.Error(w, fmt.Sprintf(`{"error": "Scrape failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if len(games) == 0 {
		log.Printf("No games found for eventid=%s, clubid=%s", eventID, clubID)
	}

	log.Printf("Returning %d games", len(games))
	json.NewEncoder(w).Encode(games)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{
		"status":      "healthy",
		"service":     "Fixed Home/Away GotSport Parser",
		"version":     "12.2",
		"timestamp":   time.Now().Format(time.RFC3339),
		"description": "Enhanced error handling and broader parsing patterns",
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
		response := "Fixed Home/Away GotSport Parser v12.2\n\n" +
			"Enhanced error handling and broader parsing patterns!\n\n" +
			"Endpoints:\n" +
			"- /health\n" +
			"- /schedule?eventid=44145&clubid=12893"
		fmt.Fprintf(w, response)
	})

	log.Printf("Fixed Home/Away GotSport Parser v12.2 starting on port %s", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server start failed: %v", err)
	}
}
