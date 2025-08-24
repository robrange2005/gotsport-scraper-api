package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
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
	log.Printf("Fetching from: %s", url)

	// Increase timeout to handle slow responses
	client := &http.Client{
		Timeout: 60 * time.Second, // Changed from 15s to 60s
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Enhanced headers to mimic a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HTTP request failed after %v: %v", time.Since(start), err)
		return nil, nil // Return nil to match original behavior
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("HTTP %d from GotSport: %s", resp.StatusCode, string(body))
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response: %v", err)
		return nil, nil
	}

	html := string(body)
	log.Printf("Retrieved %d chars from event %s in %v", len(html), eventID, time.Since(start))

	// Parse HTML with goquery
	games := parseGotSportHTMLFast(html, eventID)
	log.Printf("Found %d Reno Apex home games", len(games))

	return games, nil
}

func parseGotSportHTMLFast(html, eventID string) []Game {
	var games []Game

	// Parse HTML with goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Printf("Failed to parse HTML: %v", err)
		return games
	}

	// Find all table rows with game data
	doc.Find("tr").Each(func(i int, row *goquery.Selection) {
		// Skip empty rows
		if row.Find("td").Length() == 0 {
			return
		}

		// Check for Reno Apex home team
		homeTeam := strings.TrimSpace(row.Find("td").Eq(0).Find("a").Text())
		if !strings.Contains(strings.ToLower(homeTeam), "reno apex") {
			return
		}

		// Extract game details
		game := Game{
			HomeTeam:    homeTeam,
			AwayTeam:    strings.TrimSpace(row.Find("td").Eq(1).Find("a").Text()),
			Date:        convertDate(strings.TrimSpace(row.Find("td").Eq(2).Text())),
			Time:        strings.TrimSpace(row.Find("td").Eq(3).Text()),
			Venue:       strings.TrimSpace(row.Find("td").Eq(4).Find("a").Text()),
			Field:       strings.TrimSpace(row.Find("td").Eq(5).Find("a").Text()),
			Division:    strings.TrimSpace(row.Find("td").Eq(6).Find("a").Text()),
			Competition: strings.TrimSpace(row.Find("td").Eq(6).Find("a").Text()),
		}

		// Only include games with required fields
		if game.HomeTeam != "" && game.AwayTeam != "" && game.Date != "" && game.Time != "" {
			games = append(games, game)
			log.Printf("Parsed: %s vs %s on %s at %s", game.HomeTeam, game.AwayTeam, game.Date, game.Time)
		}
	})

	return games
}

func convertDate(dateStr string) string {
	monthMap := map[string]string{
		"Jan": "01", "Feb": "02", "Mar": "03", "Apr": "04",
		"May": "05", "Jun": "06", "Jul": "07", "Aug": "08",
		"Sep": "09", "Oct": "10", "Nov": "11", "Dec": "12",
	}

	parts := strings.Fields(strings.TrimSuffix(dateStr, ","))
	if len(parts) >= 3 {
		month := monthMap[parts[0]]
		day := parts[1]
		year := parts[2]
		if len(day) == 1 {
			day = "0" + day
		}
		if month != "" {
			return fmt.Sprintf("%s-%s-%s", year, month, day)
		}
	}
	return dateStr
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

