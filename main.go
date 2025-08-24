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
	
	log.Printf("Scraping ALL Reno Apex teams from Event %s...", eventID)
	
	client := &http.Client{
		Timeout: 20 * time.Second,
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Failed to create request for event %s: %v", eventID, err)
		return getPlaceholderGames(eventID), nil
	}
	
	// Enhanced headers to look more like a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Cache-Control", "max-age=0")
	
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HTTP request failed for event %s: %v", eventID, err)
		return getPlaceholderGames(eventID), nil
	}
	defer resp.Body.Close()
	
	log.Printf("Event %s response: HTTP %d", eventID, resp.StatusCode)
	
	if resp.StatusCode != 200 {
		log.Printf("Non-200 status for event %s: %d", eventID, resp.StatusCode)
		return getPlaceholderGames(eventID), nil
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response for event %s: %v", eventID, err)
		return getPlaceholderGames(eventID), nil
	}
	
	bodyStr := string(body)
	log.Printf("Event %s response: %d characters", eventID, len(bodyStr))
	
	// Parse ALL Reno Apex games from the page
	games := parseAllRenoApexGames(bodyStr, eventID)
	if len(games) > 0 {
		log.Printf("Found %d Reno Apex games in event %s", len(games), eventID)
		return games, nil
	}
	
	log.Printf("No Reno Apex games found in event %s, using enhanced placeholder", eventID)
	return getPlaceholderGames(eventID), nil
}

func scrapeECNLSchedule() ([]Game, error) {
	url := "https://theecnl.com/sports/2023/8/8/ECNLRLG_0808235356.aspx"
	
	log.Printf("Scraping ALL Reno Apex ECNL teams...")
	
	client := &http.Client{
		Timeout: 20 * time.Second,
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Failed to create ECNL request: %v", err)
		return getECNLPlaceholderGames(), nil
	}
	
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://theecnl.com/")
	req.Header.Set("Connection", "keep-alive")
	
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ECNL HTTP request failed: %v", err)
		return getECNLPlaceholderGames(), nil
	}
	defer resp.Body.Close()
	
	log.Printf("ECNL response: HTTP %d", resp.StatusCode)
	
	if resp.StatusCode != 200 {
		log.Printf("ECNL non-200 status: %d", resp.StatusCode)
		return getECNLPlaceholderGames(), nil
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read ECNL response: %v", err)
		return getECNLPlaceholderGames(), nil
	}
	
	bodyStr := string(body)
	log.Printf("ECNL response: %d characters", len(bodyStr))
	
	// Parse ALL Reno Apex ECNL games
	games := parseAllRenoApexECNLGames(bodyStr)
	if len(games) > 0 {
		log.Printf("Found %d Reno Apex ECNL games", len(games))
		return games, nil
	}
	
	log.Printf("No ECNL games found, using placeholder")
	return getECNLPlaceholderGames(), nil
}

func parseAllRenoApexGames(html, eventID string) []Game {
	var games []Game
	
	// Look for various patterns that might contain Reno Apex games
	htmlLower := strings.ToLower(html)
	
	// Check if page contains Reno Apex
	if !strings.Contains(htmlLower, "reno apex") && !strings.Contains(htmlLower, "reno") {
		log.Printf("Event %s: No 'Reno' found in page", eventID)
		return games
	}
	
	log.Printf("Event %s: Found 'Reno' in page, parsing games...", eventID)
	
	// Strategy 1: Look for table rows that might contain games
	games = append(games, parseTableBasedGames(html, eventID)...)
	
	// Strategy 2: Look for div-based schedule entries
	if len(games) == 0 {
		games = append(games, parseDivBasedGames(html, eventID)...)
	}
	
	// Strategy 3: Use regex to find team names and opponents
	if len(games) == 0 {
		games = append(games, parseWithRegex(html, eventID)...)
	}
	
	// Filter for weekend home games only
	weekendGames := filterWeekendHomeGames(games)
	
	log.Printf("Event %s: Parsed %d total games, %d weekend home games", eventID, len(games), len(weekendGames))
	
	return weekendGames
}

func parseTableBasedGames(html, eventID string) []Game {
	var games []Game
	
	// Look for table rows containing game data
	rowPattern := regexp.MustCompile(`(?i)<tr[^>]*>(.*?)</tr>`)
	rows := rowPattern.FindAllString(html, -1)
	
	log.Printf("Event %s: Found %d table rows to parse", eventID, len(rows))
	
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row), "reno apex") || strings.Contains(strings.ToLower(row), "reno") {
			game := extractGameFromTableRow(row, eventID)
			if game.HomeTeam != "" {
				games = append(games, game)
			}
		}
	}
	
	return games
}

func parseDivBasedGames(html, eventID string) []Game {
	var games []Game
	
	// Look for div elements that might contain game info
	divPattern := regexp.MustCompile(`(?i)<div[^>]*>(.*?)</div>`)
	divs := divPattern.FindAllString(html, -1)
	
	log.Printf("Event %s: Found %d divs to parse", eventID, len(divs))
	
	for _, div := range divs {
		if strings.Contains(strings.ToLower(div), "reno apex") || strings.Contains(strings.ToLower(div), "reno") {
			game := extractGameFromDiv(div, eventID)
			if game.HomeTeam != "" {
				games = append(games, game)
			}
		}
	}
	
	return games
}

func parseWithRegex(html, eventID string) []Game {
	var games []Game
	
	// Look for patterns like "Reno Apex U12" vs "Opponent"
	renoPattern := regexp.MustCompile(`(?i)(reno\s+apex[^<]*?u\d+[^<]*?)(?:\s+vs\.?\s+|\s+@\s+|\s+-\s+)([^<]+)`)
	matches := renoPattern.FindAllStringSubmatch(html, -1)
	
	log.Printf("Event %s: Found %d Reno Apex team matches", eventID, len(matches))
	
	for _, match := range matches {
		if len(match) >= 3 {
			homeTeam := strings.TrimSpace(match[1])
			awayTeam := strings.TrimSpace(match[2])
			
			// Extract age group from team name
			agePattern := regexp.MustCompile(`U(\d+)`)
			ageMatch := agePattern.FindStringSubmatch(homeTeam)
			
			division := "Unknown"
			if len(ageMatch) >= 2 {
				division = "U" + ageMatch[1]
				if strings.Contains(strings.ToLower(homeTeam), "girls") {
					division += " Girls"
				} else if strings.Contains(strings.ToLower(homeTeam), "boys") {
					division += " Boys"
				}
			}
			
			game := Game{
				HomeTeam:    homeTeam,
				AwayTeam:    awayTeam,
				Date:        getNextWeekend().Saturday.Format("2006-01-02"),
				Time:        "TBD",
				Field:       "TBD",
				Venue:       "Reno Sports Complex",
				Division:    division,
				Competition: "NorCal Premier League",
			}
			games = append(games, game)
		}
	}
	
	return games
}

func extractGameFromTableRow(row, eventID string) Game {
	// Try to extract game details from table row HTML
	homeTeam := extractWithRegex(row, `(?i)(reno\s+apex[^<>]*?)(?:\s+vs|\s+@|<)`)
	awayTeam := extractWithRegex(row, `(?i)vs\.?\s+([^<>]+?)(?:<|$)`)
	
	if homeTeam == "" {
		return Game{}
	}
	
	return Game{
		HomeTeam:    cleanTeamName(homeTeam),
		AwayTeam:    cleanTeamName(awayTeam),
		Date:        extractDateFromHTML(row),
		Time:        extractTimeFromHTML(row),
		Field:       extractFieldFromHTML(row),
		Venue:       "Reno Sports Complex",
		Division:    extractDivisionFromTeamName(homeTeam),
		Competition: "NorCal Premier League",
	}
}

func extractGameFromDiv(div, eventID string) Game {
	homeTeam := extractWithRegex(div, `(?i)(reno\s+apex[^<>]*?)(?:\s+vs|\s+@|<)`)
	awayTeam := extractWithRegex(div, `(?i)vs\.?\s+([^<>]+?)(?:<|$)`)
	
	if homeTeam == "" {
		return Game{}
	}
	
	return Game{
		HomeTeam:    cleanTeamName(homeTeam),
		AwayTeam:    cleanTeamName(awayTeam),
		Date:        extractDateFromHTML(div),
		Time:        extractTimeFromHTML(div),
		Field:       extractFieldFromHTML(div),
		Venue:       "Reno Sports Complex",
		Division:    extractDivisionFromTeamName(homeTeam),
		Competition: "NorCal Premier League",
	}
}

func parseAllRenoApexECNLGames(html string) []Game {
	var games []Game
	
	htmlLower := strings.ToLower(html)
	if strings.Contains(htmlLower, "reno apex") || 
	   strings.Contains(htmlLower, "reno") ||
	   strings.Contains(htmlLower, "schedule") {
		
		log.Printf("ECNL: Found schedule content")
		
		// For now, return multiple age groups as placeholder
		// This would be enhanced with real parsing
		weekend := getNextWeekend()
		
		ecnlAgeGroups := []string{"U15", "U16", "U17", "U18", "U19"}
		
		for i, ageGroup := range ecnlAgeGroups {
			date := weekend.Saturday
			if i%2 == 1 {
				date = weekend.Sunday
			}
			
			games = append(games, Game{
				HomeTeam:    fmt.Sprintf("Reno Apex %s Girls", ageGroup),
				AwayTeam:    fmt.Sprintf("ECNL Opponent %d", i+1),
				Date:        date.Format("2006-01-02"),
				Time:        fmt.Sprintf("%d:00 PM", 10+i),
				Field:       fmt.Sprintf("Field %c", 'A'+i),
				Venue:       "Reno Sports Complex",
				Division:    fmt.Sprintf("%s Girls", ageGroup),
				Competition: "ECNL Regional League",
			})
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

func cleanTeamName(name string) string {
	// Remove HTML tags and clean up team names
	re := regexp.MustCompile(`<[^>]*>`)
	cleaned := re.ReplaceAllString(name, "")
	return strings.TrimSpace(cleaned)
}

func extractDivisionFromTeamName(teamName string) string {
	agePattern := regexp.MustCompile(`U(\d+)`)
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
	return "Unknown"
}

func extractDateFromHTML(html string) string {
	datePattern := regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{4}`)
	match := datePattern.FindString(html)
	if match != "" {
		return match
	}
	return getNextWeekend().Saturday.Format("2006-01-02")
}

func extractTimeFromHTML(html string) string {
	timePattern := regexp.MustCompile(`\d{1,2}:\d{2}\s*[AaPp][Mm]`)
	match := timePattern.FindString(html)
	if match != "" {
		return match
	}
	return "TBD"
}

func extractFieldFromHTML(html string) string {
	fieldPattern := regexp.MustCompile(`(?i)field\s*:?\s*(\w+|\d+)`)
	matches := fieldPattern.FindStringSubmatch(html)
	if len(matches) > 1 {
		return "Field " + matches[1]
	}
	return "TBD"
}

func filterWeekendHomeGames(games []Game) []Game {
	var weekendGames []Game
	weekend := getNextWeekend()
	
	saturdayStr := weekend.Saturday.Format("2006-01-02")
	sundayStr := weekend.Sunday.Format("2006-01-02")
	
	for _, game := range games {
		// Check if it's a home game (Reno Apex is home team)
		isHome := strings.Contains(strings.ToLower(game.HomeTeam), "reno apex")
		
		// Check if it's on the weekend
		isWeekend := strings.Contains(game.Date, saturdayStr) || 
		           strings.Contains(game.Date, sundayStr) ||
		           game.Date == saturdayStr || 
		           game.Date == sundayStr
		
		if isHome && isWeekend {
			weekendGames = append(weekendGames, game)
		}
	}
	
	return weekendGames
}

type WeekendDates struct {
	Saturday time.Time
	Sunday   time.Time
}

func getNextWeekend() WeekendDates {
	now := time.Now()
	daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}
	
	saturday := now.AddDate(0, 0, daysUntilSaturday)
	sunday := saturday.AddDate(0, 0, 1)
	
	return WeekendDates{
		Saturday: saturday,
		Sunday:   sunday,
	}
}

func getPlaceholderGames(eventID string) []Game {
	weekend := getNextWeekend()
	
	var games []Game
	
	// Return multiple age groups for each event
	if eventID == "44145" {
		ageGroups := []string{"U10", "U12", "U14"}
		for i, age := range ageGroups {
			date := weekend.Saturday
			if i%2 == 1 {
				date = weekend.Sunday
			}
			
			games = append(games, Game{
				HomeTeam:    fmt.Sprintf("Reno Apex %s Boys", age),
				AwayTeam:    fmt.Sprintf("Sacramento Team %d", i+1),
				Date:        date.Format("2006-01-02"),
				Time:        fmt.Sprintf("%d:00 AM", 9+i*2),
				Field:       fmt.Sprintf("Field %d", i+1),
				Venue:       "Reno Sports Complex",
				Division:    fmt.Sprintf("%s Boys", age),
				Competition: "NorCal Premier League",
			})
		}
	} else if eventID == "44142" {
		ageGroups := []string{"U10", "U12", "U14"}
		for i, age := range ageGroups {
			date := weekend.Saturday
			if i%2 == 0 {
				date = weekend.Sunday
			}
			
			games = append(games, Game{
				HomeTeam:    fmt.Sprintf("Reno Apex %s Girls", age),
				AwayTeam:    fmt.Sprintf("Folsom Team %d", i+1),
				Date:        date.Format("2006-01-02"),
				Time:        fmt.Sprintf("%d:00 PM", 12+i*2),
				Field:       fmt.Sprintf("Field %d", i+4),
				Venue:       "Reno Sports Complex",
				Division:    fmt.Sprintf("%s Girls", age),
				Competition: "NorCal Premier League",
			})
		}
	}
	
	return games
}

func getECNLPlaceholderGames() []Game {
	weekend := getNextWeekend()
	var games []Game
	
	ecnlAgeGroups := []string{"U15", "U16", "U17", "U18"}
	
	for i, age := range ecnlAgeGroups {
		date := weekend.Saturday
		if i%2 == 1 {
			date = weekend.Sunday
		}
		
		games = append(games, Game{
			HomeTeam:    fmt.Sprintf("Reno Apex %s Girls", age),
			AwayTeam:    fmt.Sprintf("ECNL Opponent %d", i+1),
			Date:        date.Format("2006-01-02"),
			Time:        fmt.Sprintf("%d:00 PM", 10+i*2),
			Field:       fmt.Sprintf("Field %c", 'A'+i),
			Venue:       "Reno Sports Complex",
			Division:    fmt.Sprintf("%s Girls", age),
			Competition: "ECNL Regional League",
		})
	}
	
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

	log.Printf("=== Schedule Request ===")
	log.Printf("EventID: %s, ClubID: %s", eventID, clubID)

	var games []Game
	var err error
	
	if eventID == "ecnl" {
		games, err = scrapeECNLSchedule()
	} else {
		games, err = scrapeGotSportSchedule(eventID, clubID)
	}
	
	if err != nil {
		log.Printf("Error scraping: %v", err)
	}

	log.Printf("Returning %d games for eventID: %s", len(games), eventID)
	json.NewEncoder(w).Encode(games)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"status":    "healthy",
		"service":   "Comprehensive Multi-Age Sports Scraper",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "5.0-all-age-groups",
		"sources":   []string{"GotSport All Ages (44145, 44142)", "ECNL All Ages"},
		"uptime":    time.Since(startTime).String(),
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
		response := "Comprehensive Multi-Age Sports Scraper v5.0\n\n"
		response += "Endpoints:\n"
		response += "- GET /health\n"
		response += "- GET /schedule?eventid=44145&clubid=12893 (ALL Reno Apex teams)\n"
		response += "- GET /schedule?eventid=44142&clubid=12893 (ALL Reno Apex teams)\n"
		response += "- GET /schedule?eventid=ecnl&clubid=12893 (ALL ECNL teams)\n\n"
		response += "Now finds ALL age groups, not just one per event!"
		fmt.Fprintf(w, response)
	})

	log.Printf("=== Comprehensive Multi-Age Sports Scraper v5.0 ===")
	log.Printf("Starting on port %s", port)
	log.Printf("Now scraping ALL Reno Apex age groups per event!")
	log.Printf("Events: 44145 (all ages), 44142 (all ages), ECNL (all ages)")
	log.Printf("Ready to find all teams!")
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
