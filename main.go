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

func getNextWeekendDates() (string, string) {
	now := time.Now()
	daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}
	
	nextSaturday := now.AddDate(0, 0, daysUntilSaturday)
	nextSunday := nextSaturday.AddDate(0, 0, 1)
	
	saturdayStr := nextSaturday.Format("Jan 02, 2006")
	sundayStr := nextSunday.Format("Jan 02, 2006")
	
	log.Printf("Looking for weekend dates: %s and %s", saturdayStr, sundayStr)
	return saturdayStr, sundayStr
}

func scrapeGotSportSchedule(eventID, clubID string) ([]Game, error) {
	url := fmt.Sprintf("https://system.gotsport.com/org_event/events/%s/schedules?club=%s", eventID, clubID)
	log.Printf("Fetching: %s", url)
	
	client := &http.Client{Timeout: 15 * time.Second}
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
	
	saturdayStr, sundayStr := getNextWeekendDates()
	htmlLower := strings.ToLower(html)
	saturdayLower := strings.ToLower(saturdayStr)
	sundayLower := strings.ToLower(sundayStr)
	
	log.Printf("Searching for: '%s' and '%s'", saturdayLower, sundayLower)
	
	var weekendSections []string
	
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
	
	start := index - 2000
	if start < 0 {
		start = 0
	}
	end := index + 5000
	if end > len(html) {
		end = len(html)
	}
	
	return html[start:end]
}

func findRenoApexGamesInSection(section string) []Game {
	var games []Game
	lines := strings.Split(section, "\n")
	
	for _, line := range lines {
		if strings.Contains(line, "- ") && strings.Contains(strings.ToLower(line), "reno apex") {
			parts := strings.Split(line, "- ")
			if len(parts) >= 2 {
				homeTeamPart := strings.TrimSpace(parts[0])
				awayTeamPart := strings.TrimSpace(parts[1])
				
				if strings.Contains(strings.ToLower(homeTeamPart), "reno apex") {
					log.Printf("Found HOME game: %s - %s", homeTeamPart, awayTeamPart)
					
					game := extractGameFromLine(line, section)
					if game.HomeTeam != "" && !isDuplicateGame(games, game) {
						games = append(games, game)
						log.Printf("Added home game: %s vs %s at %s %s", 
							game.HomeTeam, game.AwayTeam, game.Time, game.Field)
					}
				} else if strings.Contains(strings.ToLower(awayTeamPart), "reno apex") {
					log.Printf("Skipping AWAY game: %s - %s", homeTeamPart, awayTeamPart)
				}
			}
		}
	}
	
	return games
}

func extractGameFromLine(gameLine string, fullSection string) Game {
	game := Game{}
	
	lineIndex := strings.Index(fullSection, gameLine)
	if lineIndex == -1 {
		return game
	}
	
	start := lineIndex - 500
	if start < 0 {
		start = 0
	}
	end := lineIndex + 500
	if end > len(fullSection) {
		end = len(fullSection)
	}
	
	context := fullSection[start:end]
	
	parts := strings.Split(gameLine, "- ")
	if len(parts) >= 2 {
		homeRaw := strings.TrimSpace(parts[0])
		game.HomeTeam = extractTeamNameFromText(homeRaw)
		
		awayRaw := strings.TrimSpace(parts[1])
		game.AwayTeam = extractTeamNameFromText(awayRaw)
	}
	
	game.Date = findDateInContext(context)
	game.Time = findTimeInContext(context)
	fullLocation := findFullLocationInContext(context)
	game.Field = fullLocation
	game.Venue = fullLocation
	game.Division = findDivisionFromTeamName(game.HomeTeam)
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
	
	saturdayFormatted := nextSaturday.Format("Jan 02, 2006")
	sundayFormatted := nextSunday.Format("Jan 02, 2006")
	
	if strings.Contains(context, saturdayFormatted) {
		return nextSaturday.Format("2006-01-02")
	}
	if strings.Contains(context, sundayFormatted) {
		return nextSunday.Format("2006-01-02")
	}
	
	return nextSaturday.Format("2006-01-02")
}

func findTimeInContext(context string) string {
	timePatterns := []string{
		`(\d{1,2}:\d{2})\s*(AM|PM)\s*(PDT|PST|PT)?`,
		`(\d{1,2}:\d{2})(AM|PM)`,
	}
	
	for _, pattern := range timePatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindStringSubmatch(context)
		if len(matches) >= 3 {
			time := strings.TrimSpace(matches[1])
			ampm := strings.ToUpper(strings.TrimSpace(matches[2]))
			
			timeStr := time + " " + ampm
			if len(matches) >= 4 && matches[3] != "" {
				timeStr += " " + strings.ToUpper(strings.TrimSpace(matches[3]))
			}
			
			log.Printf("Found time: %s", timeStr)
			return timeStr
		}
	}
	
	log.Printf("No time found in context")
	return "TBD"
}

func findFullLocationInContext(context string) string {
	locationPatterns := []string{
		`<a[^>]*href="[^"]*schedules\?pitch[^"]*"[^>]*>([^<]+)</a>`,
		`\*\s*([^<\n]+(?:Park|Complex|Field|Stadium|Center|Facility)[^<\n]*)`,
		`([A-Za-z0-9\s]+(Park|Complex|Field|Stadium|Center|Facility)[\s\-A-Za-z0-9]*)`,
	}
	
	for _, pattern := range locationPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(context, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				location := strings.TrimSpace(match[1])
				location = strings.Trim(location, "*.,;:")
				location = strings.TrimSpace(location)
				
				if len(location) > 5 && len(location) < 100 {
					log.Printf("Found full location: '%s'", location)
					return location
				}
			}
		}
	}
	
	lines := strings.Split(context, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "*") && 
		   (strings.Contains(line, "Park") || 
		    strings.Contains(line, "Complex") || 
		    strings.Contains(line, "Field") ||
		    strings.Contains(line, "Center")) {
			
			parts := strings.Split(line, "*")
			if len(parts) > 1 {
				location := strings.TrimSpace(parts[1])
				re := regexp.MustCompile(`<[^>]*>`)
				location = re.ReplaceAllString(location, "")
				location = strings.TrimSpace(location)
				
				if len(location) > 5 {
					log.Printf("Found location (fallback): '%s'", location)
					return location
				}
			}
		}
	}
	
	log.Printf("No location found in context")
	return "Location TBD"
}

func findDivisionFromTeamName(teamName string) string {
	if teamName == "" {
		return "League"
	}
	
	if strings.Contains(teamName, "Reno APEX Soccer Club ") {
		parts := strings.Split(teamName, "Reno APEX Soccer Club ")
		if len(parts) >= 2 {
			division := strings.TrimSpace(parts[1])
			if division != "" {
				log.Printf("Division from team name: '%s'", division)
				return division
			}
		}
	}
	
	return "League"
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
		"service":     "Fixed Home/Away GotSport Parser",
		"version":     "12.0-clean",
		"timestamp":   time.Now().Format(time.RFC3339),
		"description": "Clean syntax with accurate home game detection",
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
		response := "Fixed Home/Away GotSport Parser v12.0\n\n" +
			"Accurate home game detection!\n\n" +
			"Endpoints:\n" +
			"- /health\n" +
			"- /schedule?eventid=44145&clubid=12893"
		fmt.Fprintf(w, response)
	})

	log.Printf("Fixed Home/Away GotSport Parser v12.0 starting on port %s", port)
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server start failed: %v", err)
	}
}
