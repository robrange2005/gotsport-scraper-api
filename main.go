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
	Field       string `json:"field"`       // Now contains full location
	Venue       string `json:"venue"`       // Keep for backward compatibility, will be same as Field
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

// FIXED: Better logic to distinguish home vs away games using dash pattern
func findRenoApexGamesInSection(section string) []Game {
	var games []Game
	
	// Look for the "Team - Team" pattern which indicates home vs away
	lines := strings.Split(section, "\n")
	
	for _, line := range lines {
		// Look for lines with the dash separator pattern
		if strings.Contains(line, "- ") && strings.Contains(strings.ToLower(line), "reno apex") {
			
			// Split on the dash to get home and away teams
			parts := strings.Split(line, "- ")
			if len(parts) >= 2 {
				homeTeamPart := strings.TrimSpace(parts[0])
				awayTeamPart := strings.TrimSpace(parts[1])
				
				// Check if Reno Apex is the HOME team (before the dash)
				if strings.Contains(strings.ToLower(homeTeamPart), "reno apex") {
					log.Printf("Found HOME game: %s - %s", homeTeamPart, awayTeamPart)
					
					// Extract the full context around this line for detailed parsing
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

// Extract game details from a specific line and its context
func extractGameFromLine(gameLine string, fullSection string) Game {
	game := Game{}
	
	// Get a larger context around this specific line
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
	
	// Extract teams from the dash-separated line
	parts := strings.Split(gameLine, "- ")
	if len(parts) >= 2 {
		// Home team (before dash)
		homeRaw := strings.TrimSpace(parts[0])
		game.HomeTeam = extractTeamNameFromText(homeRaw)
		
		// Away team (after dash)  
		awayRaw := strings.TrimSpace(parts[1])
		game.AwayTeam = extractTeamNameFromText(awayRaw)
	}
	
	// Extract other details from context
	game.Date = findDateInContext(context)
	game.Time = findTimeInContext(context)
	fullLocation := findFullLocationInContext(context)
	game.Field = fullLocation  // Full location goes in Field
	game.Venue = fullLocation  // Same value for backward compatibility
	game.Division = findDivisionFromTeamName(game.HomeTeam)
	game.Competition = game.Division
	
	return game
}

// Extract clean team name from text
func extractTeamNameFromText(text string) string {
	// Remove HTML tags
	re := regexp.MustCompile(`<[^>]*>`)
	cleaned := re.ReplaceAllString(text, "")
	
	// Clean up whitespace
	cleaned = strings.TrimSpace(cleaned)
	
	// Remove any leading/trailing punctuation
	cleaned = strings.Trim(cleaned, ".,;:-")
	
	return cleaned
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
	// Multiple time patterns to try
	timePatterns := []string{
		// Pattern 1: "10:30 AM PDT" or "10:30 PM PST"
		`(\d{1,2}:\d{2})\s*(AM|PM)\s*(PDT|PST|PT)?`,
		// Pattern 2: "10:30AM" or "2:00PM" (no space)
		`(\d{1,2}:\d{2})(AM|PM)`,
	}
	
	for _, pattern := range timePatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindStringSubmatch(context)
		if len(matches) >= 3 {
			time := strings.TrimSpace(matches[1])
			ampm := strings.ToUpper(strings.TrimSpace(matches[2]))
			
			// Format with proper spacing
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

// NEW: Find the complete location string (venue + field combined)
func findFullLocationInContext(context string) string {
	// Look for location patterns in the context
	locationPatterns := []string{
		// Pattern 1: Link with schedules?pitch (most reliable)
		`<a[^>]*href="[^"]*schedules\?pitch[^"]*"[^>]*>([^<]+)</a>`,
		// Pattern 2: Text after asterisk (common GotSport pattern)
		`\*\s*([^<\n]+(?:Park|Complex|Field|Stadium|Center|Facility)[^<\n]*)`,
		// Pattern 3: General location text
		`([A-Za-z0-9\s]+(Park|Complex|Field|Stadium|Center|Facility)[\s\-A-Za-z0-9]*)`,
	}
	
	for _, pattern := range locationPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(context, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				location := strings.TrimSpace(match[1])
				// Clean up the location string
				location = strings.Trim(location, "*.,;:")
				location = strings.TrimSpace(location)
				
				if len(location) > 5 && len(location) < 100 {
					log.Printf("Found full location: '%s'", location)
					return location
				}
			}
		}
	}
	
	// Fallback: look for any venue-like text
	lines := strings.Split(context, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "*") && 
		   (strings.Contains(line, "Park") || 
		    strings.Contains(line, "Complex") || 
		    strings.Contains(line, "Field") ||
		    strings.Contains(line, "Center")) {
			
			// Extract text after asterisk
			parts := strings.Split(line, "*")
			if len(parts) > 1 {
				location := strings.TrimSpace(parts[1])
				// Remove HTML tags
				re := regexp.MustCompile(`<[^>]*>`)
				location = re.ReplaceAllSt
