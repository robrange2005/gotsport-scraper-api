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
	Location    string `json:"location"` // Combined field and venue
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

	// Support multiple date formats to match GotSport HTML
	saturdayFormats := []string{
		nextSaturday.Format("January 02, 2006"), // e.g., "August 30, 2025"
		nextSaturday.Format("01/02/2006"),       // e.g., "08/30/2025"
		nextSaturday.Format("Jan 02, 2006"),     // e.g., "Aug 30, 2025"
	}
	sundayFormats := []string{
		nextSunday.Format("January 02, 2006"), // e.g., "August 31, 2025"
		nextSunday.Format("01/02/2006"),      // e.g., "08/31/2025"
		nextSunday.Format("Jan 02, 2006"),    // e.g., "Aug 31, 2025"
	}

	log.Printf("Looking for weekend date patterns: Saturday %v, Sunday %v", saturdayFormats, sundayFormats)
	return saturdayFormats, sundayFormats
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

	saturdayFormats, sundayFormats := getNextWeekendDates()
	htmlLower := strings.ToLower(html)

	var weekendSections []string

	// Check for any Saturday date pattern
	for _, satPattern := range saturdayFormats {
		if strings.Contains(htmlLower, strings.ToLower(satPattern)) {
			section := extractSectionAroundDate(html, satPattern)
			if section != "" {
				weekendSections = append(weekendSections, section)
				log.Printf("Found Saturday section for %s (%d chars)", satPattern, len(section))
			}
		}
	}

	// Check for any Sunday date pattern
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
		return ""
	}

	start := index - 3000 // Increased to capture more context
	if start < 0 {
		start = 0
	}
	end := index + 6000
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

				if strings.Contains(strings.ToLower(homeTeamPart), "reno apex
