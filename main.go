// PATCH 1: Improved opponent extraction
func findOpponentInContext(context string) string {
	lines := strings.Split(context, "\n")
	
	// First pass: look for team names that are clearly opponents (not Reno)
	var candidates []string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "<a href=") && !strings.Contains(strings.ToLower(line), "reno") && !strings.Contains(line, "schedules?pitch") {
			// Extract team name from <a> tag
			if start := strings.Index(line, ">"); start != -1 {
				if end := strings.Index(line[start+1:], "<"); end != -1 {
					opponent := strings.TrimSpace(line[start+1 : start+1+end])
					// Filter out short/invalid names and ensure it looks like a team
					if len(opponent) > 8 && strings.Contains(opponent, " ") && 
					   !strings.Contains(strings.ToLower(opponent), "reno") &&
					   !strings.Contains(strings.ToLower(opponent), "field") &&
					   !strings.Contains(strings.ToLower(opponent), "park") {
						candidates = append(candidates, opponent)
					}
				}
			}
		}
	}
	
	// Return the best candidate (longest meaningful team name)
	var bestOpponent string
	for _, candidate := range candidates {
		// Prefer names that contain age indicators that match the home team
		if len(candidate) > len(bestOpponent) {
			bestOpponent = candidate
		}
	}
	
	if bestOpponent != "" {
		log.Printf("Found opponent: %s (from %d candidates)", bestOpponent, len(candidates))
		return bestOpponent
	}
	
	log.Printf("No opponent found in context")
	return "TBD"
}

// PATCH 2: Better time formatting  
func findTimeInContext(context string) string {
	// Multiple time patterns to try
	timePatterns := []string{
		// Pattern 1: "10:30 AM PDT" or "10:30 PM PST"
		`(\d{1,2}:\d{2})\s*(AM|PM)\s*(PDT|PST|PT)?`,
		// Pattern 2: "10:30AM" or "2:00PM" (no space)
		`(\d{1,2}:\d{2})(AM|PM)`,
		// Pattern 3: Just the time "10:30" followed by AM/PM
		`(\d{1,2}:\d{2})[^\w]*([AP]M)`,
		// Pattern 4: Time in 24-hour format "14:30"
		`(\d{2}:\d{2})`,
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
			
			log.Printf("Found time with pattern '%s': %s", pattern, timeStr)
			return timeStr
		} else if len(matches) >= 2 {
			timeStr := strings.TrimSpace(matches[0])
			// Add space if missing between time and AM/PM
			re2 := regexp.MustCompile(`(\d{1,2}:\d{2})(AM|PM)`)
			timeStr = re2.ReplaceAllString(timeStr, "$1 $2")
			
			log.Printf("Found time with pattern '%s': %s", pattern, timeStr)
			return timeStr
		}
	}
	
	// Fallback: look for any time-like pattern manually
	words := strings.Fields(context)
	for i, word := range words {
		if strings.Contains(word, ":") && len(word) <= 6 {
			// Check if next word is AM/PM
			if i+1 < len(words) {
				next := strings.ToUpper(words[i+1])
				if strings.HasPrefix(next, "AM") || strings.HasPrefix(next, "PM") {
					timeResult := word + " " + strings.Fields(next)[0]
					log.Printf("Found time manually: %s", timeResult)
					return timeResult
				}
			}
			// Check if AM/PM is attached to the time
			if strings.HasSuffix(strings.ToUpper(word), "AM") || strings.HasSuffix(strings.ToUpper(word), "PM") {
				// Add space between time and AM/PM
				re := regexp.MustCompile(`(\d{1,2}:\d{2})(AM|PM)`)
				timeResult := re.ReplaceAllString(strings.ToUpper(word), "$1 $2")
				log.Printf("Found attached time: %s", timeResult)
				return timeResult
			}
		}
	}
	
	log.Printf("No time found in context")
	return "TBD"
}

// PATCH 3: Extract complete division exactly as it appears in team name
func findDivisionInContext(context string) string {
	// First try to extract from the Reno team name itself
	lines := strings.Split(context, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "reno apex") {
			// Extract the team name from <a> tags or text
			teamName := ""
			if start := strings.Index(line, ">"); start != -1 {
				if end := strings.Index(line[start+1:], "<"); end != -1 {
					teamName = strings.TrimSpace(line[start+1 : start+1+end])
				}
			}
			
			// If no <a> tag, try to find it in the line text
			if teamName == "" {
				teamName = line
			}
			
			if teamName != "" && strings.Contains(strings.ToLower(teamName), "reno apex") {
				// Extract everything after "Reno APEX Soccer Club "
				parts := strings.Split(teamName, "Reno APEX Soccer Club ")
				if len(parts) >= 2 {
					division := strings.TrimSpace(parts[1])
					if division != "" {
						log.Printf("Found complete division from team name: '%s'", division)
						return division
					}
				}
				
				// Alternative pattern: extract everything after "Reno Apex "
				parts = strings.Split(teamName, "Reno Apex ")
				if len(parts) >= 2 {
					division := strings.TrimSpace(parts[1])
					if division != "" {
						log.Printf("Found complete division from team name (alt): '%s'", division)
						return division
					}
				}
			}
		}
	}
	
	// Fallback: look for complete division patterns in context
	divisionPatterns := []string{
		// Complete patterns with year and direction
		`(\d{4}[BG][\w\s]*(?:NPL|Elite|Premier|Gold|Silver|Bronze)[\w\s]*(?:East|West|North|South|Central)?)`,
		// Patterns with U and direction  
		`(U\d{2}[\w\s]*(?:NPL|Elite|Premier|Gold|Silver|Bronze)[\w\s]*(?:East|West|North|South|Central)?)`,
		// Patterns with 2-digit and direction
		`(\d{2}[BG][\w\s]*(?:NPL|Elite|Premier|Gold|Silver|Bronze)[\w\s]*(?:East|West|North|South|Central)?)`,
		// Just the league types
		`(Premier[\w\s]*(?:East|West|North|South|Central)?)`,
		`(NPL[\w\s]*(?:East|West|North|South|Central)?)`,
		`(Elite[\w\s]*(?:East|West|North|South|Central)?)`,
		`(Gold[\w\s]*(?:East|West|North|South|Central)?)`,
		`(Silver[\w\s]*(?:East|West|North|South|Central)?)`,
	}
	
	for _, pattern := range divisionPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindStringSubmatch(context)
		if len(matches) >= 2 {
			division := strings.TrimSpace(matches[1])
			log.Printf("Found division with pattern: '%s'", division)
			return division
		}
	}
	
	// Final fallback checks
	contextLower := strings.ToLower(context)
	if strings.Contains(contextLower, "premier") {
		return "Premier"
	}
	if strings.Contains(contextLower, "npl") {
		return "NPL"
	}
	if strings.Contains(contextLower, "elite") {
		return "Elite"
	}
	if strings.Contains(contextLower, "gold") {
		return "Gold"
	}
	
	return "League"
}
