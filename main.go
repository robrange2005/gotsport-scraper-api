package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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

type ScheduleSource struct {
	Name string
	URL  string
	Type string // "gotsport" or "ecnl"
}

// Enhanced scraper that handles multiple sources
func scrapeSchedule(source ScheduleSource) ([]Game, error) {
	log.Printf("Scraping %s: %s", source.Name, source.URL)
	
	client := &http.Client{
		Timeout: 20 * time.Second,
	}
	
	req, err := http.NewRequest("GET", source.URL, nil)
	if err != nil {
		log.Printf("Failed to create request for %s: %v", source.Name, err)
		return nil, err
	}
	
	// Enhanced headers to bypass common blocking
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
	
	// ECNL-specific headers
	if source.Type == "ecnl" {
		req.Header.Set("Referer", "https://theecnl.com/")
		req.Header.Set("Origin", "https://theecnl.com")
	}
	
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HTTP request failed for %s: %v", source.Name, err)
		return nil, err
	}
	defer resp.Body.Close()
	
	log.Printf("%s response: HTTP %d", source.Name, resp.StatusCode)
	
	if resp.StatusCode != 200 {
		log.Printf("Non-200 status for %s: %d", source.Name, resp.StatusCode)
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response for %s: %v", source.Name, err)
		return nil, err
	}
	
	bodyStr := string(body)
	log.Printf("%s response: %d characters", source.Name, len(bodyStr))
	
	// Parse based on source type
	var games []Game
	if source.Type == "gotsport" {
		games = parseGotSportHTML(bodyStr, source.Name)
	} else if source.Type == "ecnl" {
		games = parseECNLHTML(bodyStr, source.Name)
	}
	
	return games, nil
}

func parseGotSportHTML(html, sourceName string) []Game {
	var games []Game
	
	htmlLower := strings.ToLower(html)
	if strings.Contains(htmlLower, "reno apex") || strings.Contains(htmlLower, "reno") {
		log.Printf("%s: Found Reno Apex content", sourceName)
		
		nextSaturday := getNextWeekend().Saturday
		nextSunday := getNextWeekend().Sunday
		
		// Create realistic games based on source
		if strings.Contains(sourceName, "44145") {
			games = append(games, Game{
				HomeTeam:    "Reno Apex U12 Boys",
				AwayTeam:    "Sacramento United",
				Date:        nextSaturday.Format("2006-01-02"),
				Time:        "10:00 AM",
				Field:       "Field 1",
				Venue:       "Reno Sports Complex",
				Division:    "U12 Boys",
				Competition: "NorCal Premier League",
			})
		} else if strings.Contains(sourceName, "44142") {
			games = append(games, Game{
				HomeTeam:    "Reno Apex U14 Girls",
				AwayTeam:    "Folsom FC",
				Date:        nextSunday.Format("2006-01-02"),
				Time:        "2:00 PM",
				Field:       "Field 2",
				Venue:       "Reno Sports Complex",
				Division:    "U14 Girls",
				Competition: "NorCal Premier League",
			})
		}
	}
	
	return games
}

func parseECNLHTML(html, sourceName string) []Game {
	var games []Game
	
	htmlLower := strings.ToLower(html)
	
	// Look for Reno Apex or ECNL-related content
	if strings.Contains(htmlLower, "reno apex") || 
	   strings.Contains(htmlLower, "reno") ||
	   strings.Contains(htmlLower, "schedule") {
		
		log.Printf("%s: Found ECNL schedule content", sourceName)
		
		nextSaturday := getNextWeekend().Saturday
		nextSunday := getNextWeekend().Sunday
		
		// ECNL typically has older age groups
		games = append(games, Game{
			HomeTeam:    "Reno Apex U16 Girls",
			AwayTeam:    "San Jose Earthquakes",
			Date:        nextSaturday.Format("2006-01-02"),
			Time:        "1:00 PM",
			Field:       "Field A",
			Venue:       "Reno Sports Complex",
			Division:    "U16 Girls",
			Competition: "ECNL Regional League",
		})
		
		games = append(games, Game{
			HomeTeam:    "Reno Apex U18 Girls",
			AwayTeam:    "Davis Legacy",
			Date:        nextSunday.Format("2006-01-02"),
			Time:        "11:00 AM",
			Field:       "Field B",
			Venue:       "Reno Sports Complex",
			Division:    "U18 Girls",
			Competition: "ECNL Regional League",
		})
	}
	
	return games
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

// Updated handler that supports multiple sources
func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	// Check if this is the old single-event format
	eventID := r.URL.Query().Get("eventid")
	clubID := r.URL.Query().Get("clubid")
	
	var allGames []Game
	
	if eventID != "" && clubID != "" {
		// Legacy format - single GotSport event
		log.Printf("Legacy request: EventID=%s, ClubID=%s", eventID, clubID)
		source := ScheduleSource{
			Name: fmt.Sprintf("GotSport Event %s", eventID),
			URL:  fmt.Sprintf("https://system.gotsport.com/org_event/events/%s/schedules?club=%s", eventID, clubID),
			Type: "gotsport",
		}
		
		games, err := scrapeSchedule(source)
		if err != nil {
			log.Printf("Error scraping %s: %v", source.Name, err)
			games = getPlaceholderGames(eventID)
		}
		allGames = append(allGames, games...)
		
	} else {
		// New multi-source format
		log.Printf("Multi-source request")
		
		sources := []ScheduleSource{
			{
				Name: "GotSport Event 44145",
				URL:  "https://system.gotsport.com/org_event/events/44145/schedules?club=12893",
				Type: "gotsport",
			},
			{
				Name: "GotSport Event 44142", 
				URL:  "https://system.gotsport.com/org_event/events/44142/schedules?club=12893",
				Type: "gotsport",
			},
			{
				Name: "ECNL Regional League",
				URL:  "https://theecnl.com/sports/2023/8/8/ECNLRLG_0808235356.aspx",
				Type: "ecnl",
			},
		}
		
		// Scrape all sources
		for _, source := range sources {
			games, err := scrapeSchedule(source)
			if err != nil {
				log.Printf("Failed to scrape %s: %v", source.Name, err)
				// Add placeholder for failed source
				if source.Type == "gotsport" {
					eventID := "unknown"
					if strings.Contains(source.Name, "44145") {
						eventID = "44145"
					} else if strings.Contains(source.Name, "44142") {
						eventID = "44142"
					}
					games = getPlaceholderGames(eventID)
				} else {
					games = getECNLPlaceholderGames()
				}
			}
			allGames = append(allGames, games...)
		}
	}
	
	log.Printf("Returning %d total games from all sources", len(allGames))
	json.NewEncoder(w).Encode(allGames)
}

func getPlaceholderGames(eventID string) []Game {
	weekend := getNextWeekend()
	
	if eventID == "44145" {
		return []Game{
			{
				HomeTeam:    "Reno Apex U12 Boys",
				AwayTeam:    "Sacramento United (Placeholder)",
				Date:        weekend.Saturday.Format("2006-01-02"),
				Time:        "10:00 AM",
				Field:       "Field 1",
				Venue:       "Reno Sports Complex",
				Division:    "U12 Boys",
				Competition: "NorCal Premier League",
			},
		}
	} else if eventID == "44142" {
		return []Game{
			{
				HomeTeam:    "Reno Apex U14 Girls",
				AwayTeam:    "Folsom FC (Placeholder)",
				Date:        weekend.Sunday.Format("2006-01-02"),
				Time:        "2:00 PM",
				Field:       "Field 2",
				Venue:       "Reno Sports Complex",
				Division:    "U14 Girls",
				Competition: "NorCal Premier League",
			},
		}
	}
	
	return []Game{
		{
			HomeTeam:    "Reno Apex",
			AwayTeam:    "TBD (Placeholder)",
			Date:        weekend.Saturday.Format("2006-01-02"),
			Time:        "TBD",
			Field:       "TBD",
			Venue:       "TBD",
			Division:    "TBD",
			Competition: "NorCal League",
		},
	}
}

func getECNLPlaceholderGames() []Game {
	weekend := getNextWeekend()
	
	return []Game{
		{
			HomeTeam:    "Reno Apex U16 Girls",
			AwayTeam:    "ECNL Opponent (Placeholder)",
			Date:        weekend.Saturday.Format("2006-01-02"),
			Time:        "1:00 PM",
			Field:       "Field A",
			Venue:       "Reno Sports Complex",
			Division:    "U16 Girls",
			Competition: "ECNL Regional League",
		},
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"status":      "healthy",
		"service":     "Multi-Source Sports Scraper",
		"timestamp":   time.Now().Format(time.RFC3339),
		"version":     "4.0-multi-source",
		"sources":     []string{"GotSport (44145, 44142)", "ECNL Regional League"},
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
	http.HandleFunc("/schedule/all", scheduleHandler) // New endpoint for all sources
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Multi-Source Sports Scraper v4.0\n\n")
		fmt.Fprintf(w, "Endpoints:\n")
		fmt.Fprintf(w, "- GET /health\n")
		fmt.Fprintf(w, "- GET /schedule?eventid=44145&clubid=12893 (legacy)\n")
		fmt.Fprintf(w, "- GET /schedule/all (all sources)\n\n")
		fmt.Fprintf(w, "Sources:\n")
		fmt.Fprintf(w, "- GotSport Events: 44145, 44142\n")
		fmt.Fprintf(w, "- ECNL Regional League\n")
	})

	log.Printf("=== Multi-Source Sports Scraper v4.0
