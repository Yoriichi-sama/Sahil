package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// --- Configuration and Data Structures ---

const (
	SCHEDULE_DIR = "NEET_Schedule"
	STATE_FILE   = "schedule_state.json"
	CONFIG_FILE  = "config.json" 
	TIME_FORMAT  = "2006-01-02" 
	
	BREAK_MINUTES = 10 
	
	// Persistence File 
	PROGRESS_FILE = "session_progress.tmp"
	
	// Adaptive Scheduling Constants
	TIME_BUFFER_FACTOR = 1.45        
	REVISION_TIME_HRS = 1.5          
	MAX_REVISIONS = 3                
	
	// Timer Constants
	PROGRESS_SAVE_INTERVAL = 5 * time.Second 
)

// Config represents global scheduling parameters.
type Config struct {
	StartDate       string `json:"start_date"`
	SyllabusEndDate string `json:"syllabus_end_date"`
	ExamDate        string `json:"exam_date"`

	DailyStudyHrs   float64 `json:"daily_study_hrs"`
	MaxSessionHrs   float64 `json:"max_session_hrs"`
	WeeklyRestDay   time.Weekday `json:"weekly_rest_day"`
	DailyBufferMins int          `json:"daily_buffer_min"`
	RestDayActivity string       `json:"rest_day_activity"`
}

// Session represents a single scheduled study block for a day.
type Session struct {
	Subject    string  `json:"subject"`
	Chapter    string  `json:"chapter"`
	Duration   float64 `json:"duration"` // in hours
	Status     string  `json:"status"`   // "Pending", "Completed", "Missed"
	Type       string  `json:"type"`     // "Study", "Revision", "Rest", "Buffer"
	ChapterID  string  `json:"chapter_id,omitempty"`
}

// ChapterWorkload tracks the details of a single chapter, including revision state.
type ChapterWorkload struct {
	ID              string  `json:"id"`
	Subject         string  `json:"subject"`
	Chapter         string  `json:"chapter"`
	
	// Core Study Metrics
	RemainingTime   float64 `json:"remaining_time"`
	WeightedTime    float64 `json:"weighted_time"`
	Weightage       float64 `json:"weightage"` 
	Difficulty      float64 `json:"difficulty"` 
	PriorityScore   float64 `json:"priority_score"`
	
	// NEW Adaptive Metrics
	SuccessRate     float64 `json:"success_rate"` // 0.0 to 1.0 (0.5 default)
	Attempts        int     `json:"attempts"`     // Total sessions scheduled for this chapter
	
	// Revision Metrics
	IsStudyCompleted bool   `json:"is_study_completed"`
	RevisionCount    int    `json:"revision_count"`
	NextRevisionDate string `json:"next_revision_date"` // Date when next revision is due
	InitialRevisionIntervalDays int `json:"initial_revision_interval_days"` // Adaptive interval
}

// ScheduleState holds the persistent data for the scheduler.
type ScheduleState struct {
	Workload              map[string]ChapterWorkload `json:"workload"`
	DailyQuotaWT          float64                    `json:"daily_quota_wt"`
	LastScheduledDate     string                     `json:"last_scheduled_date"`
	TotalWeightedWorkload float64                    `json:"total_weighted_workload"`
	TotalRemainingTime    float64                    `json:"total_remaining_time"`
	NetStudyDays          int                        `json:"net_study_days"`
}

// SessionProgress stores the state of an interrupted timer using the unique ChapterID.
type SessionProgress struct {
	Date           string `json:"date"`
	ChapterID      string `json:"chapter_id"` 
	ElapsedSeconds int    `json:"elapsed_seconds"`
}

// Simplified NEET Syllabus Data for demonstration


var syllabusData = map[string]map[string]map[string]float64{
	"Physics": {
		"Units & Measurements":                         map[string]float64{"weight": 0.02, "difficulty": 2.0, "time_est_hrs": 6.0},
		"Kinematics (1D & 2D)":                         map[string]float64{"weight": 0.08, "difficulty": 3.0, "time_est_hrs": 18.0},
		"Laws of Motion & Friction":                   map[string]float64{"weight": 0.09, "difficulty": 4.0, "time_est_hrs": 14.0},
		"Work, Energy & Power":                        map[string]float64{"weight": 0.07, "difficulty": 4.0, "time_est_hrs": 12.0},
		"Centre of Mass & Collisions":                 map[string]float64{"weight": 0.03, "difficulty": 3.0, "time_est_hrs": 8.0},
		"Rotational Motion & Moment of Inertia":       map[string]float64{"weight": 0.05, "difficulty": 4.0, "time_est_hrs": 12.0},
		"Simple Harmonic Motion":                      map[string]float64{"weight": 0.03, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Fluid Mechanics":                             map[string]float64{"weight": 0.03, "difficulty": 3.0, "time_est_hrs": 8.0},
		"Thermodynamics & Kinetic Theory":             map[string]float64{"weight": 0.07, "difficulty": 5.0, "time_est_hrs": 14.0},
		"Oscillations & Waves":                        map[string]float64{"weight": 0.04, "difficulty": 3.5, "time_est_hrs": 10.0},
		"Electrostatics":                              map[string]float64{"weight": 0.07, "difficulty": 4.0, "time_est_hrs": 12.0},
		"Current Electricity":                         map[string]float64{"weight": 0.06, "difficulty": 3.5, "time_est_hrs": 10.0},
		"Magnetism & Magnetic Effects of Current":     map[string]float64{"weight": 0.05, "difficulty": 4.0, "time_est_hrs": 10.0},
		"Electromagnetic Induction & AC":              map[string]float64{"weight": 0.05, "difficulty": 4.0, "time_est_hrs": 10.0},
		"Electromagnetic Waves":                       map[string]float64{"weight": 0.02, "difficulty": 3.0, "time_est_hrs": 6.0},
		"Geometrical Optics":                          map[string]float64{"weight": 0.04, "difficulty": 3.0, "time_est_hrs": 10.0},
		"Wave Optics":                                 map[string]float64{"weight": 0.03, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Modern Physics (Photoelectric, Atomic, Nuclear)": map[string]float64{"weight": 0.06, "difficulty": 4.5, "time_est_hrs": 12.0},
		"Semiconductors & Electronic Devices":         map[string]float64{"weight": 0.03, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Communication Systems (Basics)":              map[string]float64{"weight": 0.02, "difficulty": 3.0, "time_est_hrs": 6.0},
	},

	"Chemistry": {
		// Physical
		"Basic Concepts & Stoichiometry":              map[string]float64{"weight": 0.03, "difficulty": 2.5, "time_est_hrs": 8.0},
		"Atomic Structure & Electronic Configuration": map[string]float64{"weight": 0.04, "difficulty": 3.0, "time_est_hrs": 8.0},
		"Chemical Bonding & Molecular Structure":     map[string]float64{"weight": 0.06, "difficulty": 3.5, "time_est_hrs": 15.0},
		"States of Matter (Gases, Liquids, Solids)":  map[string]float64{"weight": 0.03, "difficulty": 3.0, "time_est_hrs": 8.0},
		"Thermodynamics & Chemical Energetics":       map[string]float64{"weight": 0.05, "difficulty": 4.0, "time_est_hrs": 12.0},
		"Chemical Equilibrium (incl. Ionic & Solubility)": map[string]float64{"weight": 0.05, "difficulty": 4.0, "time_est_hrs": 10.0},
		"Chemical Kinetics":                          map[string]float64{"weight": 0.03, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Electrochemistry":                           map[string]float64{"weight": 0.03, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Surface Chemistry":                           map[string]float64{"weight": 0.01, "difficulty": 2.5, "time_est_hrs": 4.0},

		// Inorganic
		"Periodic Table & Periodicity":               map[string]float64{"weight": 0.03, "difficulty": 3.0, "time_est_hrs": 6.0},
		"Hydrogen & Its Compounds":                   map[string]float64{"weight": 0.01, "difficulty": 2.5, "time_est_hrs": 4.0},
		"S-block Elements":                           map[string]float64{"weight": 0.02, "difficulty": 3.0, "time_est_hrs": 6.0},
		"P-block Elements":                           map[string]float64{"weight": 0.03, "difficulty": 3.0, "time_est_hrs": 10.0},
		"D & F Block Elements (Transition Metals, Lanthanoids/Actinoids)": map[string]float64{"weight": 0.03, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Coordination Compounds":                     map[string]float64{"weight": 0.02, "difficulty": 3.5, "time_est_hrs": 6.0},
		"Extraction & Metallurgy":                    map[string]float64{"weight": 0.01, "difficulty": 2.5, "time_est_hrs": 4.0},
		"Qualitative Inorganic Analysis":             map[string]float64{"weight": 0.02, "difficulty": 3.0, "time_est_hrs": 6.0},

		// Organic
		"Basics of Organic Chemistry & Mechanisms":   map[string]float64{"weight": 0.04, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Hydrocarbons (Alkanes, Alkenes, Alkynes, Aromatics)": map[string]float64{"weight": 0.06, "difficulty": 4.0, "time_est_hrs": 12.0},
		"Haloalkanes & Haloarenes":                   map[string]float64{"weight": 0.02, "difficulty": 3.0, "time_est_hrs": 6.0},
		"Alcohols, Phenols & Ethers":                 map[string]float64{"weight": 0.03, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Aldehydes, Ketones & Carboxylic Acids":      map[string]float64{"weight": 0.03, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Amines & Diazonium Chemistry":               map[string]float64{"weight": 0.03, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Biomolecules (Carbs, Proteins, Lipids, Vitamins)": map[string]float64{"weight": 0.03, "difficulty": 3.0, "time_est_hrs": 8.0},
		"Polymers & Practical Chemistry":             map[string]float64{"weight": 0.01, "difficulty": 2.5, "time_est_hrs": 4.0},
	},

	"Biology": {
		// Botany
		"Diversity of Living Organisms (Classification)": map[string]float64{"weight": 0.03, "difficulty": 3.0, "time_est_hrs": 6.0},
		"Plant Kingdom & Morphology":                  map[string]float64{"weight": 0.02, "difficulty": 2.5, "time_est_hrs": 6.0},
		"Cell Structure & Cell Cycle":                 map[string]float64{"weight": 0.04, "difficulty": 3.0, "time_est_hrs": 6.0},
		"Plant Physiology (Transport, Nutrition, Growth)": map[string]float64{"weight": 0.05, "difficulty": 3.5, "time_est_hrs": 10.0},
		"Photosynthesis & Respiration (Plant)":        map[string]float64{"weight": 0.04, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Plant Reproduction & Development":            map[string]float64{"weight": 0.03, "difficulty": 3.0, "time_est_hrs": 6.0},

		// Zoology / Human biology
		"Human Physiology: Circulatory System & Immunity": map[string]float64{"weight": 0.06, "difficulty": 4.0, "time_est_hrs": 12.0},
		"Human Physiology: Respiratory System":        map[string]float64{"weight": 0.03, "difficulty": 3.0, "time_est_hrs": 6.0},
		"Excretory & Endocrine Systems":              map[string]float64{"weight": 0.04, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Nervous System & Sense Organs":               map[string]float64{"weight": 0.05, "difficulty": 4.0, "time_est_hrs": 10.0},
		"Human Reproduction & Reproductive Health":    map[string]float64{"weight": 0.04, "difficulty": 3.5, "time_est_hrs": 8.0},
		"Genetics & Evolution (Mendelian + Molecular)": map[string]float64{"weight": 0.06, "difficulty": 5.0, "time_est_hrs": 18.0},
		"Biotechnology & Its Applications":            map[string]float64{"weight": 0.03, "difficulty": 3.5, "time_est_hrs": 6.0},
		"Ecology & Environment (Ecosystems, Conservation)": map[string]float64{"weight": 0.04, "difficulty": 3.0, "time_est_hrs": 8.0},
		"Diversity of Animals (Invertebrates & Vertebrates)": map[string]float64{"weight": 0.02, "difficulty": 2.5, "time_est_hrs": 6.0},
		"Practical Skills, Diagrams & Experimental Interpretation": map[string]float64{"weight": 0.02, "difficulty": 3.0, "time_est_hrs": 8.0},
	},
}


var rawConfig Config

// --- Persistence Utility Functions (Configuration) ---

// saveConfig writes the current configuration to the JSON file.
func saveConfig(config Config) {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		fmt.Printf("[ERROR] Failed to encode config: %v\n", err)
		return
	}
	err = os.WriteFile(CONFIG_FILE, data, 0644)
	if err != nil {
		fmt.Printf("[ERROR] Failed to save config to %s: %v\n", CONFIG_FILE, err)
		return
	}
}

// loadConfig reads the configuration from the JSON file or returns defaults.
func loadConfig() Config {
	// Default configuration (used if config.json is not found)
	defaultConfig := Config{
		StartDate:       time.Now().Format(TIME_FORMAT), 
		SyllabusEndDate: "2026-06-30",
		ExamDate:        "2026-07-28",

		DailyStudyHrs:   6.0,
		MaxSessionHrs:   1.0,
		WeeklyRestDay:   time.Sunday, 
		DailyBufferMins: 30,
		RestDayActivity: "Recovery",
	}

	data, err := os.ReadFile(CONFIG_FILE)
	if err == nil {
		var config Config
		err = json.Unmarshal(data, &config)
		if err == nil {
			config.StartDate = time.Now().Format(TIME_FORMAT)
			return config
		}
		fmt.Printf("[ERROR] Could not decode JSON config file: %v. Using defaults.\n", err)
	} else if !os.IsNotExist(err) {
		fmt.Printf("[ERROR] Could not read config file: %v. Using defaults.\n", err)
	}
	
	return defaultConfig
}

// --- Persistence Utility Functions (Progress) ---

// loadProgress attempts to read the progress file.
func loadProgress(today time.Time) (SessionProgress, bool) {
	data, err := os.ReadFile(PROGRESS_FILE)
	if err != nil {
		return SessionProgress{}, false 
	}

	var progress SessionProgress
	if err := json.Unmarshal(data, &progress); err != nil {
		fmt.Printf("[WARNING] Corrupted progress file (%s). Deleting it.\n", PROGRESS_FILE)
		deleteProgress()
		return SessionProgress{}, false
	}
	
	// Only load if the progress is for today's date
	if progress.Date != today.Format(TIME_FORMAT) {
		deleteProgress()
		return SessionProgress{}, false
	}

	return progress, true
}

// saveProgress writes the current running session's state to the progress file.
func saveProgress(chapterID string, elapsedSeconds int) {
	today := time.Now().Truncate(24 * time.Hour)
	progress := SessionProgress{
		Date: today.Format(TIME_FORMAT),
		ChapterID: chapterID, 
		ElapsedSeconds: elapsedSeconds,
	}

	data, err := json.Marshal(progress)
	if err != nil {
		fmt.Printf("[ERROR] Failed to encode progress: %v\n", err)
		return
	}
	err = os.WriteFile(PROGRESS_FILE, data, 0644)
	if err != nil {
		fmt.Printf("[ERROR] Failed to save progress to %s: %v\n", PROGRESS_FILE, err)
	}
}

// deleteProgress removes the temporary file after successful completion/miss.
func deleteProgress() {
	if err := os.Remove(PROGRESS_FILE); err != nil && !os.IsNotExist(err) {
		fmt.Printf("[WARNING] Failed to clean up progress file %s: %v\n", PROGRESS_FILE, err)
	}
}

// --- Persistence Utility Functions (State) ---

// loadState reads the persistent state from the JSON file.
func loadState() ScheduleState {
	state := ScheduleState{Workload: make(map[string]ChapterWorkload)}
	data, err := os.ReadFile(STATE_FILE)
	if err == nil {
		err = json.Unmarshal(data, &state)
		if err == nil {
			if state.Workload == nil {
				state.Workload = make(map[string]ChapterWorkload)
			}
			if state.LastScheduledDate == "" {
				state.LastScheduledDate = time.Now().Format(TIME_FORMAT)
			}
			return state
		}
		fmt.Printf("[ERROR] Could not decode JSON state file: %v. Starting fresh.\n", err)
	} else if !os.IsNotExist(err) {
		fmt.Printf("[ERROR] Could not read state file: %v. Starting fresh.\n", err)
	}
	
	state.LastScheduledDate = time.Now().Format(TIME_FORMAT)
	return state
}

// saveState writes the current state to the JSON file.
func saveState(state ScheduleState) {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		fmt.Printf("[ERROR] Failed to encode state: %v\n", err)
		return
	}
	err = os.WriteFile(STATE_FILE, data, 0644)
	if err != nil {
		fmt.Printf("[ERROR] Failed to save state to %s: %v\n", STATE_FILE, err)
		return
	}
}

// writeDayPlan writes the plan for a specific date to a text file.
func writeDayPlan(date time.Time, sessions []Session) {
	if err := os.MkdirAll(SCHEDULE_DIR, os.ModePerm); err != nil {
		fmt.Printf("[CRITICAL ERROR] Failed to create directory '%s': %v\n", SCHEDULE_DIR, err)
		return
	}
	
	filepath := filepath.Join(SCHEDULE_DIR, date.Format(TIME_FORMAT)+".txt")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DATE: %s\n\n", date.Format(TIME_FORMAT)))

	for i, session := range sessions {
		header := fmt.Sprintf("SESSION %d:", i+1)
		if session.Type == "Buffer" || session.Type == "Rest" {
			header = "BUFFER/REST:"
		}
		sb.WriteString(fmt.Sprintf("%s\n", header))
		sb.WriteString(fmt.Sprintf("  Subject: %s\n", session.Subject))
		sb.WriteString(fmt.Sprintf("  Chapter: %s\n", session.Chapter))
		sb.WriteString(fmt.Sprintf("  Duration: %.1f hrs\n", session.Duration))
		sb.WriteString(fmt.Sprintf("  Status: %s\n", session.Status))
		sb.WriteString(fmt.Sprintf("  Type: %s\n", session.Type))
		if session.ChapterID != "" {
			sb.WriteString(fmt.Sprintf("  ID: %s\n", session.ChapterID))
		}
		sb.WriteString("\n")
	}

	err := os.WriteFile(filepath, []byte(sb.String()), 0644)
	if err != nil {
		fmt.Printf("[ERROR] Failed to write plan for %s: %v\n", date.Format(TIME_FORMAT), err)
	}
}

// readDayPlan parses a day plan file and returns a list of Session objects.
func readDayPlan(date time.Time) ([]Session, error) {
	filepath := filepath.Join(SCHEDULE_DIR, date.Format(TIME_FORMAT)+".txt")
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("could not read plan file for %s: %w", date.Format(TIME_FORMAT), err)
	}

	content := string(data)
	sessions := []Session{}

	blocks := strings.Split(content, "\n\n")

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, "DATE:") {
			continue
		}

		session := Session{Status: "Pending"} 
		
		scanner := bufio.NewScanner(strings.NewReader(block))
		
		scanner.Scan() 

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "Subject":
				session.Subject = value
			case "Chapter":
				session.Chapter = value
			case "Duration":
				durationStr := strings.TrimSuffix(value, " hrs")
				session.Duration, _ = strconv.ParseFloat(durationStr, 64)
			case "Status":
				session.Status = value
			case "Type":
				session.Type = value
			case "ID":
				session.ChapterID = value
			}
		}
		
		if session.Type != "" {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

// --- Adaptive Scheduler Functions ---

// calculateInitialRevisionInterval determines the first SR interval based on difficulty.
func calculateInitialRevisionInterval(difficulty float64) int {
	difficultyFactor := 5.0 - difficulty 
	initialIntervalDays := 7 + int(difficultyFactor * 3.0) 
	return initialIntervalDays
}

// updateChapterPerformance Adjusts a chapter's performance metrics based on the outcome of a session.
func updateChapterPerformance(wl ChapterWorkload, success bool) ChapterWorkload {
	// 1. Update Attempts and SuccessRate
	if wl.Attempts == 0 {
		wl.SuccessRate = 0.5 
	}
	
	newAttempts := wl.Attempts + 1
	var delta float64 
	
	if success {
		delta = 1.0 
		// Decrease difficulty slightly on success (min 1.0)
		wl.Difficulty = math.Max(1.0, wl.Difficulty - 0.1) 
	} else {
		delta = 0.0 
		// Increase difficulty more significantly on failure (max 5.0)
		wl.Difficulty = math.Min(5.0, wl.Difficulty + 0.3) 
	}
	
	// Rolling average for SuccessRate
	oldSum := wl.SuccessRate * float64(wl.Attempts)
	newSuccessRate := (oldSum + delta) / float64(newAttempts)
	
	wl.SuccessRate = newSuccessRate
	wl.Attempts = newAttempts
	
	// 2. Update Priority Score
	// New formula: (Weight * 0.4) + (Difficulty * 0.3) + ((1 - SuccessRate) * 0.3)
	wl.PriorityScore = (wl.Weightage * 0.4) + (wl.Difficulty * 0.3) + ((1.0 - wl.SuccessRate) * 0.3)
	
	return wl
}

// calculateQuotas initializes/updates workload and determines the daily quotas.
func calculateQuotas(state *ScheduleState) []ChapterWorkload {
	totalWeightedWorkload := 0.0
	totalRemainingTime := 0.0
	var allChapters []ChapterWorkload

	for subject, chapters := range syllabusData {
		for chapter, data := range chapters {
			chapterID := fmt.Sprintf("%s.%s", subject, chapter)
			
			wl, ok := state.Workload[chapterID]
			if !ok {
				initialTime := data["time_est_hrs"] * TIME_BUFFER_FACTOR
				initialDifficulty := data["difficulty"]

				wl = ChapterWorkload{
					ID: chapterID,
					Subject: subject,
					Chapter: chapter,
					RemainingTime: initialTime,
					Weightage: data["weight"],
					Difficulty: initialDifficulty,
					SuccessRate: 0.5, // NEW: Initial value
					Attempts: 0,       // NEW: Initial value
					IsStudyCompleted: false, 
					RevisionCount: 0,
					NextRevisionDate: "",
					InitialRevisionIntervalDays: calculateInitialRevisionInterval(initialDifficulty),
				}
			}
            
            // Recalculate Priority Score and Weighted Time for all chapters
            // This ensures the report and scheduler use the most current priority based on performance
			if wl.Attempts == 0 {
				wl.PriorityScore = (wl.Weightage * 0.6) + (wl.Difficulty * 0.4)
			} else {
				wl.PriorityScore = (wl.Weightage * 0.4) + (wl.Difficulty * 0.3) + ((1.0 - wl.SuccessRate) * 0.3)
			}


			if !wl.IsStudyCompleted && wl.RemainingTime > 0.001 {
				weightedTime := wl.RemainingTime * (1 + wl.Difficulty/5.0) * (wl.Weightage * 2.0)
				wl.WeightedTime = weightedTime
				
				totalWeightedWorkload += weightedTime
				totalRemainingTime += wl.RemainingTime
			} else {
				wl.WeightedTime = 0.0
			}
			
			allChapters = append(allChapters, wl)
			state.Workload[chapterID] = wl
		}
	}
	
	currentDate, _ := time.Parse(TIME_FORMAT, state.LastScheduledDate)
	syllabusEndDate, _ := time.Parse(TIME_FORMAT, rawConfig.SyllabusEndDate)

	if currentDate.After(syllabusEndDate) {
		currentDate = syllabusEndDate 
	}
	
	netStudyDays := 0
	for d := currentDate; d.Before(syllabusEndDate.AddDate(0, 0, 1)); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != rawConfig.WeeklyRestDay {
			netStudyDays++
		}
	}
	
	dailyQuotaWT := 0.0
	if netStudyDays > 0 {
		dailyQuotaWT = totalWeightedWorkload / float64(netStudyDays)
	} else if totalWeightedWorkload > 0 {
		dailyQuotaWT = totalWeightedWorkload 
	}

	state.TotalWeightedWorkload = totalWeightedWorkload
	state.TotalRemainingTime = totalRemainingTime
	state.NetStudyDays = netStudyDays
	state.DailyQuotaWT = dailyQuotaWT
	
	return allChapters
}

// prioritizeChapters sorts chapters by Priority Score.
func prioritizeChapters(chapters []ChapterWorkload) []ChapterWorkload {
	// Priority score calculation is handled in calculateQuotas, ensure sorting here.
	sort.Slice(chapters, func(i, j int) bool {
		return chapters[i].PriorityScore > chapters[j].PriorityScore
	})
	return chapters
}

// getDueRevisions returns a list of chapters that are ready for revision today.
func getDueRevisions(state ScheduleState, today time.Time) []ChapterWorkload {
	var dueRevisions []ChapterWorkload
	
	for _, wl := range state.Workload {
		if wl.IsStudyCompleted && wl.RevisionCount < MAX_REVISIONS && wl.NextRevisionDate != "" {
			revDate, err := time.Parse(TIME_FORMAT, wl.NextRevisionDate)
			if err == nil && !revDate.After(today) {
				dueRevisions = append(dueRevisions, wl)
			}
		}
	}
	return dueRevisions
}

// generateSchedule creates the daily plan files up to the syllabus end date.
func generateSchedule() {
	fmt.Println("--- Starting Schedule Generation ---")

	state := loadState()
	
	allChapters := calculateQuotas(&state)
	allChapters = prioritizeChapters(allChapters)
	
	currentDate, _ := time.Parse(TIME_FORMAT, state.LastScheduledDate)
	syllabusEndDate, _ := time.Parse(TIME_FORMAT, rawConfig.SyllabusEndDate)

	if state.TotalRemainingTime <= 0.001 && len(getDueRevisions(state, currentDate)) == 0 && currentDate.After(syllabusEndDate) {
		fmt.Println("[SUCCESS] All chapters are studied and all revisions are up-to-date. No new schedule generated.")
		return
	}

	fmt.Printf("[INFO] Required Daily Quota (WT): %.2f | Regenerating from %s\n", state.DailyQuotaWT, currentDate.Format(TIME_FORMAT))
	
	var activeStudyChapters []*ChapterWorkload
	for i := range allChapters {
		if !allChapters[i].IsStudyCompleted && allChapters[i].RemainingTime > 0.001 {
			activeStudyChapters = append(activeStudyChapters, &allChapters[i])
		}
	}
	
	for currentDate.Before(syllabusEndDate.AddDate(0, 0, 1)) {
		
		dailySessions := []Session{}
		dailyProgressWT := 0.0
		dailyTotalStudyHrs := rawConfig.DailyStudyHrs - (float64(rawConfig.DailyBufferMins) / 60.0)
		hoursAssigned := 0.0
		lastSubject := "" 

		if currentDate.Weekday() == rawConfig.WeeklyRestDay {
			dailySessions = append(dailySessions, Session{
				Subject:  "Rest",
				Chapter:  rawConfig.RestDayActivity,
				Duration: rawConfig.DailyStudyHrs,
				Type:     "Rest",
				Status:   "Pending",
			})
		} else {
			
			dueRevisions := getDueRevisions(state, currentDate)
			sort.Slice(dueRevisions, func(i, j int) bool {
				return dueRevisions[i].PriorityScore > dueRevisions[j].PriorityScore
			})

			for len(dueRevisions) > 0 && hoursAssigned < dailyTotalStudyHrs {
				revChapter := dueRevisions[0]
				revDuration := math.Min(REVISION_TIME_HRS, dailyTotalStudyHrs - hoursAssigned)

				if revDuration <= 0.001 {
					break 
				}

				dailySessions = append(dailySessions, Session{
					Subject:   revChapter.Subject,
					Chapter:   fmt.Sprintf("%s (Revision #%d)", revChapter.Chapter, revChapter.RevisionCount+1),
					Duration:  revDuration,
					ChapterID: revChapter.ID,
					Type:      "Revision",
					Status:    "Pending",
				})
				
				hoursAssigned += revDuration
				
				// Revisions scheduled today must update their next due date immediately for future days' planning
				// Note: We update the *plan* state here. The *actual* state update happens in runStudyTimer upon completion.
				revChapter.RevisionCount++ 
				if revChapter.RevisionCount < MAX_REVISIONS {
					nextInterval := revChapter.InitialRevisionIntervalDays * (revChapter.RevisionCount + 1)
					revChapter.NextRevisionDate = currentDate.AddDate(0, 0, nextInterval).Format(TIME_FORMAT)
				} else {
					revChapter.NextRevisionDate = "" 
				}
				state.Workload[revChapter.ID] = revChapter
				
				dueRevisions = dueRevisions[1:] 
			}
			
			var currentActive []*ChapterWorkload
			for _, ch := range activeStudyChapters {
				if !ch.IsStudyCompleted && ch.RemainingTime > 0.001 {
					currentActive = append(currentActive, ch)
				}
			}
			activeStudyChapters = currentActive 
			
			for dailyProgressWT < state.DailyQuotaWT && hoursAssigned < dailyTotalStudyHrs && len(activeStudyChapters) > 0 {
				
				foundChapterIndex := -1
				
				// Prioritize chapter not equal to the last subject (Subject Rotation Constraint)
				for i, ch := range activeStudyChapters {
					if ch.Subject != lastSubject {
						foundChapterIndex = i
						break
					}
				}
				
				if foundChapterIndex == -1 {
					foundChapterIndex = 0 // Fall back to the highest priority if rotation not possible
				}
				
				currentChapter := activeStudyChapters[foundChapterIndex]
				
				sessionDuration := math.Min(rawConfig.MaxSessionHrs, currentChapter.RemainingTime)
				if hoursAssigned+sessionDuration > dailyTotalStudyHrs {
					sessionDuration = dailyTotalStudyHrs - hoursAssigned
				}
				
				if sessionDuration <= 0.001 {
					break 
				}

				sessionWT := sessionDuration * (1 + currentChapter.Difficulty/5.0) * (currentChapter.Weightage * 2.0)
				
				dailySessions = append(dailySessions, Session{
					Subject:   currentChapter.Subject,
					Chapter:   currentChapter.Chapter,
					Duration:  sessionDuration,
					ChapterID: currentChapter.ID,
					Type:      "Study",
					Status:    "Pending",
				})

				dailyProgressWT += sessionWT
				hoursAssigned += sessionDuration
				lastSubject = currentChapter.Subject 
				
				currentChapter.RemainingTime -= sessionDuration
				
				if currentChapter.RemainingTime <= 0.001 { 
					currentChapter.IsStudyCompleted = true
					currentChapter.NextRevisionDate = currentDate.AddDate(0, 0, currentChapter.InitialRevisionIntervalDays).Format(TIME_FORMAT)
					
					activeStudyChapters = append(activeStudyChapters[:foundChapterIndex], activeStudyChapters[foundChapterIndex+1:]...)
					sort.Slice(activeStudyChapters, func(i, j int) bool {
						return activeStudyChapters[i].PriorityScore > activeStudyChapters[j].PriorityScore
					})
				}
				state.Workload[currentChapter.ID] = *currentChapter
			}

			dailySessions = append(dailySessions, Session{
				Subject:  "Buffer",
				Chapter:  "Recovery/Review",
				Duration: float64(rawConfig.DailyBufferMins) / 60.0,
				Type:     "Buffer",
				Status:   "Pending",
			})
		}
		
		writeDayPlan(currentDate, dailySessions)
		currentDate = currentDate.AddDate(0, 0, 1)
		state.LastScheduledDate = currentDate.Format(TIME_FORMAT)
	}
	
	saveState(state)
	fmt.Println("\n--- Schedule Generation Complete ---")
	fmt.Printf("Syllabus plans saved in the '%s/' directory until %s.\n", SCHEDULE_DIR, syllabusEndDate.Format(TIME_FORMAT))
}

// processMissedSessionsForDate loads a day's plan, marks pending study/revision sessions as "Missed", and returns them.
func processMissedSessionsForDate(date time.Time) ([]Session, error) {
	sessions, err := readDayPlan(date)
	if err != nil {
		return nil, err
	}
	
	missedSessions := []Session{}
	updated := false

	for i, s := range sessions {
		if s.Status == "Pending" && (s.Type == "Study" || s.Type == "Revision") {
			sessions[i].Status = "Missed"
			missedSessions = append(missedSessions, sessions[i])
			updated = true
		}
	}

	if updated {
		writeDayPlan(date, sessions)
	}

	return missedSessions, nil
}

// adjustWorkload incorporates missed work and triggers a schedule regeneration.
func adjustWorkload(missedSessions []Session, auditDate time.Time) {
	fmt.Println("\n[ADJUSTMENT] Recalculating workload due to missed sessions...")
	state := loadState()
	
	if len(state.Workload) == 0 {
		fmt.Println("[WARNING] No active workload in state. Skipping adjustment.")
		return
	}

	// 1. Update workload, difficulty, and Success Rate for missed sessions
	for _, session := range missedSessions {
		chID := session.ChapterID
		duration := session.Duration

		if chID != "" {
			if workload, ok := state.Workload[chID]; ok {
                
                // NEW: Update performance based on failure (sets success=false)
                workload = updateChapterPerformance(workload, false) 
                
				if session.Type == "Revision" {
					// Pushes revision back one day, resetting the count decrement (which updateChapterPerformance already handled)
					workload.NextRevisionDate = auditDate.AddDate(0, 0, 1).Format(TIME_FORMAT) 
					workload.RevisionCount-- 
					workload.RevisionCount = int(math.Max(0, float64(workload.RevisionCount)))
					fmt.Printf("  -> Missed Revision for %s. Resetting due date (SR: %.2f).\n", workload.Chapter, workload.SuccessRate)
				} else { 
					// Adds time back to the remaining time
					workload.RemainingTime += duration
					fmt.Printf("  -> Added %.1f hrs back to initial study of %s (New Priority: %.2f).\n", duration, workload.Chapter, workload.PriorityScore)
				}
				state.Workload[chID] = workload
			}
		}
	}
    
    // 2. DYNAMIC DAILY HOUR ADJUSTMENT (Only triggered if a significant number of sessions were missed)
    if len(missedSessions) > 2 {
        if rawConfig.DailyStudyHrs > 4.0 {
            rawConfig.DailyStudyHrs = math.Max(4.0, rawConfig.DailyStudyHrs - 0.5)
            fmt.Printf("\n[AUTOPILOT] Due to %d missed study/revision sessions on %s, Daily Study Hours were automatically **REDUCED to %.1f hrs** to prevent burnout.\n", len(missedSessions), auditDate.Format(TIME_FORMAT), rawConfig.DailyStudyHrs)
            saveConfig(rawConfig)
        } else {
            fmt.Println("\n[AUTOPILOT] Significant sessions missed, but daily hours are already at minimum (4.0 hrs). No further reduction.")
        }
    }

	restartDate := auditDate.AddDate(0, 0, 1) 
	state.LastScheduledDate = restartDate.Format(TIME_FORMAT)
	
	saveState(state)
	
	fmt.Printf("[ADJUSTMENT] Re-generating schedule from %s with adjusted workload...\n", restartDate.Format(TIME_FORMAT))
	generateSchedule()
	fmt.Println("[ADJUSTMENT] Schedule successfully updated and re-balanced.")
}

// A simple structure to pass commands from the input routine
type command struct {
	action string
}

// inputReader runs in a separate goroutine and sends commands non-blockingly.
func inputReader(cmdChan chan<- command, stopChan <-chan bool) {
	reader := bufio.NewReader(os.Stdin)
	for {
		select {
		case <-stopChan:
			return 
		default:
		}
		
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		
		if input != "" {
			select {
			case cmdChan <- command{action: input}:
			case <-stopChan:
				return
			default: 
				// If cmdChan is full, skip the command to prevent blocking the input routine.
			}
		}
	}
}

// runStudyTimer implements the interactive study timer utility with persistence.
func runStudyTimer(sessions []Session, sessionIndex int, initialElapsed int, today time.Time) (bool, []Session) {
	
	session := &sessions[sessionIndex]
	totalSeconds := int(session.Duration * 3600)
	elapsedSeconds := initialElapsed
	
	var startTime time.Time
	
	if initialElapsed == 0 {
		startTime = time.Now()
		fmt.Printf("\n[START] Starting %s session for %.1f hrs (Total: %d seconds). Press 'p' to pause.\n", session.Type, session.Duration, totalSeconds)
	} else {
		startTime = time.Now().Add(time.Duration(-initialElapsed) * time.Second)
		fmt.Printf("\n[RESUME] Resuming %s session. %s/%s complete. Press 'p' to pause.\n", session.Type, time.Duration(initialElapsed)*time.Second, time.Duration(totalSeconds)*time.Second)
        // FIX: Immediate redraw on resume
        remaining := totalSeconds - elapsedSeconds
        fmt.Printf("\r[TIMER] %s - Remaining: %s | Status: RUNNING  ", session.Chapter, time.Duration(remaining)*time.Second)
	}
	
	paused := false
	missedSessions := []Session{}
	
	ticker := time.NewTicker(time.Second) 
	saveTicker := time.NewTicker(PROGRESS_SAVE_INTERVAL)
	stopTimerChan := make(chan bool) 
	stopInputChan := make(chan bool) 
	cmdChan := make(chan command, 1) 
	
	go inputReader(cmdChan, stopInputChan) 

	// Persistence Goroutine
	go func() {
		for {
			select {
			case <-saveTicker.C:
				if !paused && elapsedSeconds < totalSeconds && session.ChapterID != "" {
					saveProgress(session.ChapterID, elapsedSeconds)
				}
			case <-stopTimerChan:
				saveTicker.Stop()
				return
			}
		}
	}()

	finished := false
	for elapsedSeconds < totalSeconds && !finished {
		
		select {
		case cmd := <-cmdChan:
			switch cmd.action {
			case "p":
				if !paused {
					paused = true
					fmt.Print("\n[ACTION] Paused. Enter 'r' to resume, 'f' to finish early, or 'm' to mark missed. ")
					if session.ChapterID != "" {
						saveProgress(session.ChapterID, elapsedSeconds) 
					}
				}
			case "r":
				if paused {
					paused = false
					startTime = time.Now().Add(time.Duration(-elapsedSeconds) * time.Second)
					
					// FIX: Immediately update the timer display upon resume
					remaining := totalSeconds - elapsedSeconds
					fmt.Printf("\r[TIMER] %s - Remaining: %s | Status: RUNNING  ", session.Chapter, time.Duration(remaining)*time.Second)
				}
			case "f":
				session.Status = "Completed"
				fmt.Println("\n[ACTION] Session finished early/forced completion.")
				finished = true
			case "m":
				session.Status = "Missed"
				missedSessions = append(missedSessions, *session)
				fmt.Println("\n[ACTION] Session marked as MISSED. This will be rescheduled.")
				finished = true
			default:
				// Only print help message if paused, otherwise ignore input
				if paused {
					fmt.Print("Invalid command. Options: p, r, f, m. ")
				}
			}
		
		case <-ticker.C:
			if !paused {
				elapsedSeconds = int(time.Since(startTime).Seconds())
			}

			remaining := totalSeconds - elapsedSeconds
			
			// Display update
			if elapsedSeconds%10 == 0 || elapsedSeconds == 1 || remaining <= 5 {
				status := "RUNNING"
				if paused { status = "PAUSED" }
				fmt.Printf("\r[TIMER] %s - Remaining: %s | Status: %s   ", session.Chapter, time.Duration(remaining)*time.Second, status)
			}
			
			if remaining <= 0 {
				finished = true
				break
			}
		}
	} 
	
	// Clean up goroutines
	close(stopInputChan) 
	close(stopTimerChan)
	ticker.Stop()
	
	if session.Status != "Missed" {
		session.Status = "Completed"
		if elapsedSeconds >= totalSeconds {
			fmt.Println("\n\n[COMPLETED] Session finished! Great job. 🔔")
		}
		
		// Update persistent workload state upon completion
		if session.ChapterID != "" {
			state := loadState()
			if workload, ok := state.Workload[session.ChapterID]; ok {
				
				// Update performance metrics (Success=true)
				workload = updateChapterPerformance(workload, true) 
				
				if session.Type == "Revision" {
					
					if workload.RevisionCount < MAX_REVISIONS {
						// Exponentially spaced revision interval based on initial setting
						nextInterval := workload.InitialRevisionIntervalDays * (workload.RevisionCount + 1)
						workload.NextRevisionDate = today.AddDate(0, 0, nextInterval).Format(TIME_FORMAT)
					} else {
						workload.NextRevisionDate = "" 
					}
					// Increment the count in the persistent state only on completion
					workload.RevisionCount++ 
				} else {
					// Deduct time for initial study
					workload.RemainingTime = math.Max(0, workload.RemainingTime - session.Duration) 
					if workload.RemainingTime <= 0.001 {
						workload.IsStudyCompleted = true
						// First revision interval
						workload.NextRevisionDate = today.AddDate(0, 0, workload.InitialRevisionIntervalDays).Format(TIME_FORMAT)
					}
				}
				
				state.Workload[session.ChapterID] = workload
				saveState(state)
			}
		}

		deleteProgress() 
		writeDayPlan(today, sessions) 
		return true, sessions
	}
	
	// Handle Missed session flow
	deleteProgress()
	writeDayPlan(today, sessions) 
	adjustWorkload(missedSessions, today)
	
	return true, sessions
}

// runBreakTimer implements the automatic break timer utility.
func runBreakTimer(durationMins int) {
	totalSeconds := durationMins * 60
	elapsedSeconds := 0
	startTime := time.Now()
	paused := false
	
	ticker := time.NewTicker(time.Second)
	stopInputChan := make(chan bool) 
	cmdChan := make(chan command, 1) 
	
	go inputReader(cmdChan, stopInputChan)
	
	fmt.Printf("\n[BREAK] Starting %d minute break. Press 'q' to skip, 'p' to pause. ☕️\n", durationMins)
	
	for elapsedSeconds < totalSeconds {
		select {
		case cmd := <-cmdChan:
			switch cmd.action {
			case "q":
				fmt.Println("\n[ACTION] Break skipped.")
				elapsedSeconds = totalSeconds // Exit loop
			case "p":
				if !paused {
					paused = true
					fmt.Print("\n[ACTION] Break Paused. Enter 'r' to resume. ")
				}
			case "r":
				if paused {
					paused = false
					startTime = time.Now().Add(time.Duration(-elapsedSeconds) * time.Second)
					fmt.Print("\n[ACTION] Break Resumed. ")
				}
			}
		case <-ticker.C:
			if !paused {
				elapsedSeconds = int(time.Since(startTime).Seconds())
			}
			remaining := totalSeconds - elapsedSeconds
			
			if elapsedSeconds%15 == 0 || elapsedSeconds == 1 || remaining <= 5 {
				status := "RUNNING"
				if paused { status = "PAUSED" }
				fmt.Printf("\r[TIMER] Break Remaining: %s | Status: %s ", time.Duration(remaining)*time.Second, status)
			}

			if remaining <= 0 {
				break
			}
		}
	}
	
	ticker.Stop()
	close(stopInputChan)
	
	if elapsedSeconds >= totalSeconds {
		fmt.Println("\n\n[BREAK] Break finished! Time to select your next session.")
	}
}

// runTimerCLI implements the interactive timer utility for study sessions.
func runTimerCLI() {
	realToday := time.Now().Truncate(24 * time.Hour)
	fmt.Printf("\n--- Timer CLI for %s ---\n", realToday.Format(TIME_FORMAT))

	// 1. Rollover Check (Audit past days for missed sessions)
	state := loadState()
	lastScheduled, _ := time.Parse(TIME_FORMAT, state.LastScheduledDate)
	
	missedSessionsAcrossDays := []Session{}
	// Check all days from the day after the last scheduled date up to yesterday
	for d := lastScheduled.AddDate(0, 0, 1); d.Before(realToday); d = d.AddDate(0, 0, 1) {
		missed, err := processMissedSessionsForDate(d)
		if err == nil && len(missed) > 0 {
			fmt.Printf("[AUDIT] Found %d missed sessions on %s. Adjusting workload.\n", len(missed), d.Format(TIME_FORMAT))
			missedSessionsAcrossDays = append(missedSessionsAcrossDays, missed...)
		}
	}
	
	if len(missedSessionsAcrossDays) > 0 {
		fmt.Printf("[RE-BALANCING] Total %d missed sessions detected. Adjusting workload and regenerating path from TODAY (%s)...\n", len(missedSessionsAcrossDays), realToday.Format(TIME_FORMAT))
		adjustWorkload(missedSessionsAcrossDays, realToday.AddDate(0, 0, -1)) 
	} else if lastScheduled.Before(realToday) {
		fmt.Println("[RE-BALANCING] Schedule is behind. Regenerating path to ensure today is planned.")
		state.LastScheduledDate = realToday.Format(TIME_FORMAT) // Force regeneration from today
		saveState(state)
		generateSchedule()
	}

	
	sessions, err := readDayPlan(realToday)
	if err != nil {
		fmt.Printf("[ERROR] Could not load today's schedule. Run '3' (RE-GENERATE) first: %v\n", err)
		return
	}

	// 2. Resume Check
	progress, foundProgress := loadProgress(realToday)
	reader := bufio.NewReader(os.Stdin) 
	
	if foundProgress {
		sessionIndexToResume := -1
		for i, s := range sessions {
			if s.ChapterID == progress.ChapterID {
				sessionIndexToResume = i
				break
			}
		}

		if sessionIndexToResume != -1 {
			sessionToResume := sessions[sessionIndexToResume]
			fmt.Printf("\n[RESUME ALERT] Unfinished session found for %s - %s (%s elapsed).\n", 
				sessionToResume.Subject, sessionToResume.Chapter, time.Duration(progress.ElapsedSeconds)*time.Second)
			
			fmt.Print("Do you want to **resume** this session? (y/N): ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))
			
			if input == "y" {
				finished, updatedSessions := runStudyTimer(sessions, sessionIndexToResume, progress.ElapsedSeconds, realToday)
				sessions = updatedSessions
				if finished && (sessions[sessionIndexToResume].Type == "Study" || sessions[sessionIndexToResume].Type == "Revision") && sessions[sessionIndexToResume].Status == "Completed" {
					runBreakTimer(BREAK_MINUTES)
				}
			} else {
				fmt.Println("\n[ACTION] Marking interrupted session as MISSED and rescheduling.")
				sessions[sessionIndexToResume].Status = "Missed"
				writeDayPlan(realToday, sessions)
				adjustWorkload([]Session{sessions[sessionIndexToResume]}, realToday)
				deleteProgress()
				sessions, _ = readDayPlan(realToday) // Reload
			}
		} else {
			fmt.Println("[WARNING] Progress file found but chapter ID mismatch. Deleting progress file.")
			deleteProgress()
		}
	}
	
	for {
		// Display Sessions
		fmt.Println("\n-- Today's Schedule --")
		hasPending := false
		for i, s := range sessions {
			if s.Type == "Study" || s.Type == "Revision" {
				hasPending = hasPending || (s.Status == "Pending")
				status := s.Status
				fmt.Printf("[%d] %.1f hrs | %s: %s (%s)\n", i+1, s.Duration, s.Subject, s.Chapter, status)
			}
		}
		
		if !hasPending {
			fmt.Println("\n[INFO] All Study/Revision sessions complete for today. Press 'q' to quit.")
		}

		fmt.Print("\n> Enter session number to START, 'm' to mark all PENDING as MISSED, 's' to see all, or 'q' to quit: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "q" {
			break
		}
		
		if input == "s" {
			fmt.Println("\n-- All Sessions (Including Buffer/Rest) --")
			for i, s := range sessions {
				fmt.Printf("[%d] %.1f hrs | %s: %s (%s, %s)\n", i+1, s.Duration, s.Subject, s.Chapter, s.Type, s.Status)
			}
			continue
		}
		
		if input == "m" && hasPending {
			missed := []Session{}
			missedCount := 0
			for i, s := range sessions {
				if s.Status == "Pending" && (s.Type == "Study" || s.Type == "Revision") {
					sessions[i].Status = "Missed"
					missed = append(missed, sessions[i])
					missedCount++
				}
			}
			if missedCount > 0 {
				writeDayPlan(realToday, sessions)
				fmt.Printf("[ACTION] Marked %d pending study/revision sessions as MISSED. These will be rescheduled.\n", missedCount)
				adjustWorkload(missed, realToday)
				sessions, _ = readDayPlan(realToday) // Reload the new schedule
			} else {
				fmt.Println("[INFO] No pending study/revision sessions to mark as missed.")
			}
			continue
		}


		sessionIndex, err := strconv.Atoi(input)
		if err != nil || sessionIndex < 1 || sessionIndex > len(sessions) {
			fmt.Println("[ERROR] Invalid input. Please enter a valid session number or command ('m', 's', 'q').")
			continue
		}

		sessionIdx := sessionIndex - 1
		session := &sessions[sessionIdx]

		if session.Status != "Pending" {
			fmt.Printf("[INFO] Session is already %s. Select another.\n", session.Status)
			continue
		}
		
		// Run the timer for the selected session (starting fresh from 0 elapsed time)
		finished, updatedSessions := runStudyTimer(sessions, sessionIdx, 0, realToday)
		sessions = updatedSessions

		if finished && (session.Type == "Study" || session.Type == "Revision") && session.Status == "Completed" {
			writeDayPlan(realToday, sessions) 
			runBreakTimer(BREAK_MINUTES)
		}
	}

	fmt.Println("\n[INFO] Exiting timer. Any unfinished session progress has been saved.")
}

// runFullReport displays the current progress and workload status.
func runFullReport() {
	fmt.Println("\n--- FULL PROGRESS REPORT ---")

	state := loadState()
	// Recalculate quotas to ensure state is fresh and prioritized
	allChapters := calculateQuotas(&state) 
	
	totalWorkload := state.TotalWeightedWorkload
	totalRemainingHrs := state.TotalRemainingTime
	netStudyDays := state.NetStudyDays
	dailyQuota := state.DailyQuotaWT
	
	if len(state.Workload) == 0 {
		fmt.Println("[INFO] No workload initialized. Please run option [3] RE-GENERATE first.")
		return
	}

	fmt.Printf("🎯 **Syllabus Target Date:** %s (Net Study Days Remaining: %d)\n", rawConfig.SyllabusEndDate, netStudyDays)
	fmt.Printf("⏳ **Total Remaining Workload:** %.2f WT (%.1f Study Hrs)\n", totalWorkload, totalRemainingHrs)
	fmt.Printf("📅 **Required Daily Quota:** %.2f WT (Weighted Time)\n", dailyQuota)
	fmt.Println("-----------------------------------------------------------------")
	
	var incompleteStudyChapters []ChapterWorkload
	var revisionDueChapters []ChapterWorkload
	var nextRevisionChapters []ChapterWorkload
	var completedChapters []ChapterWorkload

	today := time.Now().Truncate(24 * time.Hour)

	for _, wl := range allChapters {
		if !wl.IsStudyCompleted && wl.RemainingTime > 0.001 {
			incompleteStudyChapters = append(incompleteStudyChapters, wl)
		} else if wl.IsStudyCompleted && wl.RevisionCount < MAX_REVISIONS && wl.NextRevisionDate != "" {
			revDate, _ := time.Parse(TIME_FORMAT, wl.NextRevisionDate)
			if !revDate.After(today) {
				revisionDueChapters = append(revisionDueChapters, wl) // Already due
			} else {
                nextRevisionChapters = append(nextRevisionChapters, wl) // Not yet due
            }
		} else {
			completedChapters = append(completedChapters, wl)
		}
	}
	
	// Sort study chapters by priority (highest first)
	sort.Slice(incompleteStudyChapters, func(i, j int) bool {
		return incompleteStudyChapters[i].PriorityScore > incompleteStudyChapters[j].PriorityScore
	})
    
    // Sort revisions due by priority
    sort.Slice(revisionDueChapters, func(i, j int) bool {
		return revisionDueChapters[i].PriorityScore > revisionDueChapters[j].PriorityScore
	})

    // Sort upcoming revisions by date (earliest first)
    sort.Slice(nextRevisionChapters, func(i, j int) bool {
        dateI, _ := time.Parse(TIME_FORMAT, nextRevisionChapters[i].NextRevisionDate)
        dateJ, _ := time.Parse(TIME_FORMAT, nextRevisionChapters[j].NextRevisionDate)
        return dateI.Before(dateJ)
    })

	fmt.Println("\n**📚 PENDING INITIAL STUDY (Sorted by Priority)**")
	if len(incompleteStudyChapters) == 0 {
		fmt.Println("  -> All initial study complete! Time for revision phase.")
	} else {
		for _, wl := range incompleteStudyChapters {
			fmt.Printf("  - [Prio: %.2f | %.1f hrs left] %s: %s (Diff: %.1f, SR: %.2f)\n", 
				wl.PriorityScore, wl.RemainingTime, wl.Subject, wl.Chapter, wl.Difficulty, wl.SuccessRate)
		}
	}

	fmt.Println("\n**🔄 REVISIONS DUE TODAY**")
	if len(revisionDueChapters) == 0 {
		fmt.Println("  -> No revisions are currently due for today.")
	} else {
		for _, wl := range revisionDueChapters {
			fmt.Printf("  - [DUE | Rev #%d of %d] %s: %s (Priority: %.2f, SR: %.2f)\n", 
				wl.RevisionCount + 1, MAX_REVISIONS, wl.Subject, wl.Chapter, wl.PriorityScore, wl.SuccessRate)
		}
	}
    
    fmt.Println("\n**📅 UPCOMING REVISIONS**")
    if len(nextRevisionChapters) == 0 {
        fmt.Println("  -> No upcoming revisions scheduled.")
    } else {
        // Show top 3 upcoming revisions
        for i, wl := range nextRevisionChapters {
            if i >= 3 { break }
            fmt.Printf("  - [Next: %s | Rev #%d of %d] %s: %s\n", 
                wl.NextRevisionDate, wl.RevisionCount + 1, MAX_REVISIONS, wl.Subject, wl.Chapter)
        }
        if len(nextRevisionChapters) > 3 {
            fmt.Printf("  ... and %d more upcoming revisions. (See schedule files for full list)\n", len(nextRevisionChapters) - 3)
        }
    }
	
	// Print a general summary of completion
	total := float64(len(allChapters))
	completed := float64(len(completedChapters))
	studyProgress := 100.0
	if total > 0 {
		studyProgress = (completed / total) * 100
	}
	
	fmt.Println("\n-----------------------------------------------------------------")
	fmt.Printf("✅ **Overall Chapter Completion:** %.1f%% (%d of %d chapters)\n", studyProgress, len(completedChapters), int(total))
	fmt.Println("-----------------------------------------------------------------")
}


// --- CONFIGURATION INPUT LOGIC ---

func readFloat(reader *bufio.Reader, prompt string, defaultValue float64) float64 {
	fmt.Printf("%s (Current: %.1f): ", prompt, defaultValue)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	val, err := strconv.ParseFloat(input, 64)
	if err != nil {
		fmt.Println("[ERROR] Invalid number format. Using current value.")
		return defaultValue
	}
	return val
}

func readInt(reader *bufio.Reader, prompt string, defaultValue int) int {
	fmt.Printf("%s (Current: %d): ", prompt, defaultValue)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(input)
	if err != nil {
		fmt.Println("[ERROR] Invalid integer format. Using current value.")
		return defaultValue
	}
	return val
}

func readDate(reader *bufio.Reader, prompt string, defaultValue string) string {
	fmt.Printf("%s (Format YYYY-MM-DD, Current: %s): ", prompt, defaultValue)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	_, err := time.Parse(TIME_FORMAT, input)
	if err != nil {
		fmt.Println("[ERROR] Invalid date format. Using current value.")
		return defaultValue
	}
	return input
}

func readWeekday(reader *bufio.Reader, prompt string, defaultValue time.Weekday) time.Weekday {
	dayNames := map[string]time.Weekday{
		"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, 
		"wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday,
	}
	fmt.Printf("%s (Current: %s): ", prompt, defaultValue)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultValue
	}
	if day, ok := dayNames[input]; ok {
		return day
	}
	fmt.Println("[ERROR] Invalid day. Enter full day name (e.g., monday). Using current value.")
	return defaultValue
}

func promptConfig(currentConfig Config) Config {
	reader := bufio.NewReader(os.Stdin)
	newConfig := currentConfig

	fmt.Println("\n--- Configure Scheduler Parameters ---")
	
	newConfig.SyllabusEndDate = readDate(reader, "Syllabus Completion Target Date", newConfig.SyllabusEndDate)
	newConfig.ExamDate = readDate(reader, "Final Exam Date (for reference)", newConfig.ExamDate)

	newConfig.DailyStudyHrs = readFloat(reader, "Total Daily Study Hours (Excluding Buffer/Breaks)", newConfig.DailyStudyHrs)
	newConfig.MaxSessionHrs = readFloat(reader, "Maximum Hours per Single Session", newConfig.MaxSessionHrs)
	newConfig.DailyBufferMins = readInt(reader, "Daily Buffer/Review Time (in minutes)", newConfig.DailyBufferMins)

	newConfig.WeeklyRestDay = readWeekday(reader, "Weekly Rest Day (e.g., sunday)", newConfig.WeeklyRestDay)

	return newConfig
}

// --- MAIN MENU FUNCTION ---

func runMainMenu() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("\n--- Adaptive NEET Scheduler Menu ---")
		fmt.Println("[1] Start **TIMER CLI** (Daily Study)")
		fmt.Println("[2] View **FULL REPORT** (Syllabus Status)")
		fmt.Println("[3] **RE-GENERATE** Schedule (Initialize or Re-balance)")
		fmt.Println("[4] **CHANGE CONFIGURATION** (Dates, Times, etc.)")
		fmt.Println("[q] Quit")
		fmt.Print("\n> Enter your choice: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "1":
			runTimerCLI()
		case "2", "report":
			runFullReport()
		case "3", "generate":
			fmt.Println("\n[ACTION] Running Schedule Generation...")
			generateSchedule()
		case "4", "config":
			fmt.Println("\n[ACTION] Changing Configuration...")
			newConfig := promptConfig(rawConfig)
			rawConfig = newConfig
			saveConfig(rawConfig)
			fmt.Println("\n[INFO] Configuration updated and saved. Please RE-GENERATE the schedule (Option 3) to apply changes.")
		case "q":
			fmt.Println("\nExiting application. Goodbye! 👋")
			return
		default:
			fmt.Println("[ERROR] Invalid choice. Please enter '1', '2', '3', '4', or 'q'.")
		}
	}
}

// --- Main Execution Block ---

func main() {
	rawConfig = loadConfig()
	
	// Command-line execution for generation (e.g., `go run neet_path_builder.go generate`)
	if len(os.Args) > 1 {
		command := os.Args[1]
		if command == "generate" {
			generateSchedule()
			return
		}
	}
	
	// Interactive CLI execution (default mode)
	if _, err := os.Stat(SCHEDULE_DIR); os.IsNotExist(err) {
		fmt.Printf("[SETUP REQUIRED] The '%s' directory is missing.\n", SCHEDULE_DIR)
		fmt.Println("Please run 'go run neet_path_builder.go generate' first to create the initial schedule, or use option '3' in the menu.")
	}
	
	runMainMenu()
}