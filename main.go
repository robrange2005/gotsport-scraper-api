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
		nextSu
