// REPLACE the findRenoApexGamesInSection function with this simpler version
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
							game.HomeTeam, game.AwayTeam, game.Time, game.Venue)
					}
				} else if strings.Contains(strings.ToLower(awayTeamPart), "reno apex") {
					log.Printf("Skipping AWAY game: %s - %s", homeTeamPart, awayTeamPart)
				}
			}
		}
	}
	
	return games
}

// NEW: Extract game details from a specific line and its context
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
	game.Venue, game.Field = findVenueAndFieldInContext(context)
	game.Division = findDivisionFromTeamName(game.HomeTeam)
	game.Competition = game.Division
	
	return game
}

// NEW: Extract clean team name from text
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

// NEW: Extract division directly from team name
func findDivisionFromTeamName(teamName string) string {
	if teamName == "" {
		return "League"
	}
	
	// Extract everything after "Reno APEX Soccer Club "
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
	
	// Fallback patterns
	divisionPatterns := []string{
		`(\d{4}[BG][\w\s]*(?:NPL|Elite|Premier)[\w\s]*(?:East|West)?)`,
		`(\d{2}[BG][\w\s]*(?:NPL|Elite|Premier|PreNPL)[\w\s]*(?:\([A-Z]\))?)`,
	}
	
	for _, pattern := range divisionPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindStringSubmatch(teamName)
		if len(matches) >= 2 {
			return strings.TrimSpace(matches[1])
		}
	}
	
	return "League"
}

// Check for duplicate games (keep this function)
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
