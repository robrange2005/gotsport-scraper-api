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

	// Broad date formats based on HTML ("Aug 30, 2025")
	saturdayFormats := []string{
		nextSaturday.Format("Jan 02, 2006"),     // "Aug 30, 2025"
		nextSaturday.Format("Jan 2, 2006"),      // "Aug 30, 2025" (single digit)
		nextSaturday.Format("January 02, 2006"), // "August 30, 2025"
		nextSaturday.Format("01/02/2006"),       // "08/30/2025"
	}
	sundayFormats := []string{
		nextSunday.Format("Jan 02, 2006"),
		nextSunday.Format("Jan 2, 2006"),
		nextSunday.Format("January 02, 2006"),
		nextSunday.Format("01/02/2006"),
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

	games := parseWeekendGames(html, eventID)
	log.Printf("Found %d weekend games", len(games))
	return games, nil
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

	// If no date sections, use full HTML as fallback
	if len(weekendSections) == 0 {
		log.Printf("No date sections found, using full HTML as fallback")
		weekendSections = append(weekendSections, html)
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

func findRenoApexGamesInSection(section string) []Game {
	var games []Game

	// Regex to match <tr> rows in the schedule table
	rowPattern := regexp.MustCompile(`(?is)<tr>\s*(<td>.*?</td>)\s*(<td>.*?</td>)\s*(<td>.*?</td>)\s*(<td>.*?</td>)\s*(<td>.*?</td>)\s*(<td>.*?</td>)\s*(<td>.*?</td>)\s*</tr>`)
	matches := rowPattern.FindAllStringSubmatch(section, -1)

	for _, match := range matches {
		if len(match) >= 8 {
			matchID := cleanText(match[1])
			dateTime := cleanText(match[2])
			homeTeam := cleanText(match[3])
			results := cleanText(match[4])
			awayTeam := cleanText(match[5])
			location := cleanText(match[6])
			division := cleanText(match[7])

			if strings.Contains(strings.ToLower(homeTeam), "reno apex") && results == "-" { // Home game check
				log.Printf("Found HOME game: %s vs %s", homeTeam, awayTeam)

				game := Game{
					HomeTeam: homeTeam,
					AwayTeam: awayTeam,
					Location: location,
					Division: division,
					Competition: division,
				}

				// Parse date and time from dateTime cell (e.g., "Aug 30, 2025 1:00PM PDT")
				parsedDate, parsedTime := parseDateTime(dateTime)
				game.Date = parsedDate
				game.Time = parsedTime

				if game.Date != "" && game.Time != "TBD" && !isDuplicateGame(games, game) {
					games = append(games, game)
					log.Printf("Added game: %s vs %s at %s %s (%s)", game.HomeTeam, game.AwayTeam, game.Time, game.Location, game.Date)
				}
			}
		}
	}

	return games
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
	re := regexp.MustCompile(`(?i)([A-Za-z]+ \d{1,2}, \d{4}) ([\d:]+[AP]M [A-Za-z]+)`)
	matches := re.FindStringSubmatch(dateTime)
	if len(matches) >= 3 {
		dateStr := matches[1]
		timeStr := matches[2]

		parsedDate, err := time.Parse("Jan 02, 2006", dateStr)
		if err != nil {
			parsedDate, err = time.Parse("January 02, 2006", dateStr)
		}
		if err == nil {
			return parsedDate.Format("2006-01-02"), timeStr
		}
	}

	log.Printf("Failed to parse date/time: %s", dateTime)
	return "", "TBD"
}

func isDuplicateGame(existingGames []Game, newGame Game) bool {
	for _, existing := range existingGames {
		if existing.Date == newGame.Date &&
		   existing.Time == newGame.Time &&
		   strings.EqualFold(existing.HomeTeam, newGame.HomeTeam) {
			return true
		}
	}
	return false
}

func scrapeECNLSchedule() ([]Game, error) {
	return []Game{}, nil
}

func scheduleHandler(w http.ResponseWriter, r *http.ResponseWriter) {
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
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), 500)
		return
	}

	json.NewEncoder(w).Encode(games)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{
		"status":      "healthy",
		"service":     "Fixed Home/Away GotSport Parser",
		"version":     "12.3",
		"timestamp":   time.Now().Format(time.RFC3339),
		"description": "Table-based parsing for GotSport schedules",
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
		response := "Fixed Home/Away GotSport Parser v12.3\n\n" +
			"Table-based parsing for GotSport schedules!\n\n" +
			"Endpoints:\n" +
			"- /health\n" +
			"- /schedule?eventid=44145&clubid=12893"
		fmt.Fprintf(w, response)
	})

	log.Printf("Fixed Home/Away GotSport Parser v12.3 starting on port %s", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server start failed: %v", err)
	}
}
