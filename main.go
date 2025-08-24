package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

/* ---------- Types ---------- */

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

type scheduleReq struct {
	EventID string `json:"eventid"`
	ClubID  string `json:"clubid"`
}

/* ---------- Helpers ---------- */

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func cors(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func getPSTLocation() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.FixedZone("PDT", -7*60*60) // fallback
	}
	return loc
}

func getNextWeekendDates() ([]string, []string) {
	now := time.Now().In(getPSTLocation())
	daysUntilSaturday := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}
	nextSaturday := now.AddDate(0, 0, daysUntilSaturday)
	nextSunday := nextSaturday.AddDate(0, 0, 1)

	saturdayFormats := []string{
		nextSaturday.Format("Jan 02, 2006"),
		nextSaturday.Format("Jan 2, 2006"),
		nextSaturday.Format("January 02, 2006"),
		nextSaturday.Format("01/02/2006"),
		nextSaturday.Format("Jan. 02, 2006"),
	}
	sundayFormats := []string{
		nextSunday.Format("Jan 02, 2006"),
		nextSunday.Format("Jan 2, 2006"),
		nextSunday.Format("January 02, 2006"),
		nextSunday.Format("01/02/2006"),
		nextSunday.Format("Jan. 02, 2006"),
	}

	log.Printf("Weekend date patterns (PT): Sat %v | Sun %v", saturdayFormats, sundayFormats)
	return saturdayFormats, sundayFormats
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

/* ---------- Scraper ---------- */

func scrapeGotSportSchedule(eventID, clubID string) ([]Game, error) {
	url := fmt.Sprintf("https://system.gotsport.com/org_event/events/%s/schedules?club=%s", eventID, clubID)
	log.Printf("Fetching: %s", url)

	client := &http.Client{
		Timeout: 45 * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConns:        20,
			MaxConnsPerHost:     20,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; RenoApexScraper/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body failed: %v", err)
	}
	html := string(body)
	log.Printf("HTML length: %d chars; sample: %s ...", len(html), html[:min(len(html), 500)])

	games := parseWeekendGames(html, eventID)
	if len(games) == 0 {
		return nil, fmt.Errorf("no games found for event %s", eventID)
	}
	return games, nil
}

func parseWeekendGames(html, eventID string) []Game {
	var games []Game
	saturdayFormats, sundayFormats := getNextWeekendDates()
	htmlLower := strings.ToLower(html)

	var weekendSections []string
	for _, sat := range saturdayFormats {
		if strings.Contains(htmlLower, strings.ToLower(sat)) {
			if s := extractSectionAroundDate(html, sat); s != "" {
				weekendSections = append(weekendSections, s)
			}
		}
	}
	for _, sun := range sundayFormats {
		if strings.Contains(htmlLower, strings.ToLower(sun {
		})) {
			if s := extractSectionAroundDate(html, sun); s != "" {
				weekendSections = append(weekendSections, s)
			}
		}
	}
	if len(weekendSections) == 0 {
		weekendSections = append(weekendSections, html)
	}

	for _, section := range weekendSections {
		sectionGames := findRenoApexGamesInSection(section, html)
		games = append(games, sectionGames...)
	}
	log.Printf("Event %s: %d weekend Reno Apex home games", eventID, len(games))
	return games
}

func extractSectionAroundDate(html, dateStr string) string {
	idx := strings.Index(strings.ToLower(html), strings.ToLower(dateStr))
	if idx == -1 {
		return ""
	}
	start := idx - 5000
	if start < 0 {
		start = 0
	}
	end := idx + 10000
	if end > len(html) {
		end = len(html)
	}
	return html[start:end]
}

func findRenoApexGamesInSection(section, fullHTML string) []Game {
	var games []Game

	rowPattern := regexp.MustCompile(`(?is)<tr[^>]*>\s*((?:<td[^>]*>.*?</td>\s*){7})</tr>`)
	rows := rowPattern.FindAllStringSubmatch(section, -1)
	log.Printf("Found %d table rows in section", len(rows))

	for i, match := range rows {
		if len(match) < 2 {
			continue
		}
		tdPattern := regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
		tds := tdPattern.FindAllStringSubmatch(match[1], -1)
		if len(tds) < 7 {
			log.Printf("Row %d has %d tds (expected 7)", i+1, len(tds))
			continue
		}

		matchID := cleanText(tds[0][1])
		dateTime := cleanText(tds[1][1])
		homeTeam := cleanText(tds[2][1])
		results := cleanText(tds[3][1])
		awayTeam := cleanText(tds[4][1])
		location := cleanText(tds[5][1])
		division := cleanText(tds[6][1])

		if strings.Contains(strings.ToLower(homeTeam), "reno apex") &&
			results == "-" && isHomeGame(matchID, homeTeam, fullHTML) {

			d, t := parseDateTime(dateTime)
			game := Game{
				HomeTeam:    homeTeam,
				AwayTeam:    awayTeam,
				Location:    location,
				Division:    division,
				Competition: division,
				Date:        d,
				Time:        t,
			}
			if game.Date != "" && game.Time != "TBD" && !isDuplicateGame(games, game) {
				games = append(games, game)
			}
		}
	}
	return games
}

func isHomeGame(matchID, homeTeam, fullHTML string) bool {
	p := regexp.MustCompile(`(?is)` + regexp.QuoteMeta(matchID) + `.*?` + regexp.QuoteMeta(homeTeam) + `\s*\(H\)`)
	return p.MatchString(fullHTML)
}

func cleanText(s string) string {
	re := regexp.MustCompile(`(?s)<.*?>`)
	out := re.ReplaceAllString(s, "")
	out = strings.TrimSpace(out)
	out = strings.Trim(out, ".,;:-")
	return out
}

func parseDateTime(dateTime string) (string, string) {
	// example: "Aug 30, 2025 1:00PM PDT"
	re := regexp.MustCompile(`(?i)([A-Za-z]+\.? \d{1,2}, \d{4})\s+([\d:]+[AP]M [A-Za-z]+)`)
	m := re.FindStringSubmatch(dateTime)
	if len(m) >= 3 {
		dateStr := m[1]
		timeStr := m[2]
		if d, err := time.ParseInLocation("Jan 02, 2006", dateStr, getPSTLocation()); err == nil {
			return d.Format("2006-01-02"), timeStr
		}
		if d, err := time.ParseInLocation("January 02, 2006", dateStr, getPSTLocation()); err == nil {
			return d.Format("2006-01-02"), timeStr
		}
		if d, err := time.ParseInLocation("Jan. 02, 2006", dateStr, getPSTLocation()); err == nil {
			return d.Format("2006-01-02"), timeStr
		}
	}
	// Fallback: next Saturday (PT)
	now := time.Now().In(getPSTLocation())
	add := (6 - int(now.Weekday()) + 7) % 7
	if add == 0 {
		add = 7
	}
	return now.AddDate(0, 0, add).Format("2006-01-02"), "TBD"
}

func isDuplicateGame(existing []Game, g Game) bool {
	for _, ex := range existing {
		if ex.Date == g.Date &&
			ex.Time == g.Time &&
			strings.EqualFold(ex.HomeTeam, g.HomeTeam) &&
			strings.EqualFold(ex.AwayTeam, g.AwayTeam) {
			return true
		}
	}
	return false
}

/* ---------- HTTP Handlers ---------- */

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		// /schedule?eventid=44145&clubid=12893
		eventID := r.URL.Query().Get("eventid")
		clubID := r.URL.Query().Get("clubid")
		handleSchedule(w, r, eventID, clubID)

	case http.MethodPost:
		// JSON: {"eventid":"44145","clubid":"12893"}
		var req scheduleReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Error:  "invalid_request",
				Detail: "Body must be JSON with eventid and clubid",
			})
			return
		}
		handleSchedule(w, r, req.EventID, req.ClubID)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{
			Error:  "method_not_allowed",
			Detail: "Use GET with query or POST with JSON",
		})
	}
}

func handleSchedule(w http.ResponseWriter, r *http.Request, eventID, clubID string) {
	if eventID == "" || clubID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error:  "missing_parameters",
			Detail: "eventid and clubid are required",
		})
		return
	}

	var games []Game
	var err error

	if strings.EqualFold(eventID, "ecnl") {
		games = []Game{} // TODO: implement ECNL if needed
	} else {
		games, err = scrapeGotSportSchedule(eventID, clubID)
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error:  "scrape_failed",
			Detail: err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, games)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "healthy",
		"service":     "RenoApex GotSport Parser",
		"version":     "13.0",
		"timestamp":   time.Now().Format(time.RFC3339),
		"description": "Table-based parsing with (H) check and robust HTTP/CORS support",
	})
}

/* ---------- main ---------- */

func main() {
	// Honor PORT from Render and bind to 0.0.0.0
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/schedule", scheduleHandler)
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if cors(w, r) {
			return
		}
		fmt.Fprintln(w, "RenoApex GotSport Parser v13.0\n\nEndpoints:\n- GET/POST /schedule\n- /health")
	})

	srv := &http.Server{
		Addr:         "0.0.0.0:" + port,
		Handler:      logRequests(mux),
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(l net.Listener) context.Context { return context.Background() },
	}

	log.Printf("Starting server on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s ua=%q", r.Method, r.URL.String(), r.UserAgent())
		next.ServeHTTP(w, r)
	})
}
