package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	TIME_FORMAT              = "2006-01-02"
	SCHEDULE_DIR             = "plans"
	CONFIG_FILE              = "config.json"
	STATE_FILE               = "schedule_state.json"
	PROGRESS_FILE            = "session_progess.tmp"
	REVISION_TIME_HRS        = 0.5 
	MAX_REVISIONS            = 4   

	PROGRESS_SAVE_INTERVAL   = 5 * time.Second 
	BREAK_MINUTES            = 10

	ColorReset   = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorCyan    = "\033[36m"
	ColorMagenta = "\033[35m"
)

type Config struct {
	SyllabusEndDate          string        `json:"syllabus_end_date"`
	ExamDate                 string        `json:"exam_date"`
	DailyStudyHrs            float64       `json:"daily_study_hrs"`
	MaxSessionHrs            float64       `json:"max_session_hrs"`
	DailyBufferMins          int           `json:"daily_buffer_mins"`
	WeeklyRestDay            time.Weekday  `json:"weekly_rest_day"`
	RestDayActivity          string        `json:"rest_day_activity"`
	InitialDifficultyRating  float64       `json:"initial_difficulty_rating"`
	DifficultyAdjustmentRate float64       `json:"difficulty_adjustment_rate"`
	InitialWorkload          []ChapterWorkload `json:"initial_workload"`
}

type ChapterWorkload struct {
	ID                          string  `json:"id"`
	Subject                     string  `json:"subject"`
	Chapter                     string  `json:"chapter"`
	InitialTotalTime            float64 `json:"initial_total_time"`
	Weightage                   float64 `json:"weightage"`
	InitialRevisionIntervalDays int     `json:"initial_revision_interval_days"`
	Difficulty                  float64 `json:"difficulty"` 
	RemainingTime               float64 `json:"remaining_time"`
	IsStudyCompleted            bool    `json:"is_study_completed"`
	NextRevisionDate            string  `json:"next_revision_date"`
	RevisionCount               int     `json:"revision_count"`
	PriorityScore               float64 `json:"priority_score"`
}

type ScheduleState struct {
	Workload              map[string]ChapterWorkload `json:"workload"`
	LastScheduledDate     string                     `json:"last_scheduled_date"`
	DailyQuotaWT          float64                    `json:"daily_quota_wt"`
	TotalWeightedWorkload float64                    `json:"total_weighted_workload"`
	TotalRemainingTime    float64                    `json:"total_remaining_time"`
	NetStudyDays          int                        `json:"net_study_days"`
}

type Session struct {
	Subject   string  `json:"subject"`
	Chapter   string  `json:"chapter"`
	Duration  float64 `json:"duration"`
	ChapterID string  `json:"chapter_id"`
	Type      string  `json:"type"` 
	Status    string  `json:"status"` 
}

type Progress struct {
	ChapterID      string `json:"chapter_id"`
	ElapsedSeconds int    `json:"elapsed_seconds"`
	Date           string `json:"date"`
}

type command struct {
	action string
}

var rawConfig Config
var randSource *rand.Rand

func init() {
	randSource = rand.New(rand.NewSource(time.Now().UnixNano()))
}

func startMusic() {
	files, err := os.ReadDir("study_music")
	if err != nil || len(files) == 0 {
		fmt.Println(ColorYellow + "[MUSIC] No music found in study_music/" + ColorReset)
		return
	}

	var paths []string
	for _, f := range files {
		if !f.IsDir() && (filepath.Ext(f.Name()) == ".mp3" || filepath.Ext(f.Name()) == ".flac") {
			paths = append(paths, filepath.Join("study_music", f.Name()))
		}
	}

	if len(paths) == 0 {
		fmt.Println(ColorYellow + "[MUSIC] No mp3 files found in study_music/" + ColorReset)
		return
	}

	fmt.Println(ColorMagenta + "[MUSIC] Playing all files in study_music/ (looped)" + ColorReset)
	
	// --loop=inf will loop the playlist indefinitely
	cmd := exec.Command("mpv", append([]string{"--no-video", "--quiet", "--loop=inf"}, paths...)...)
	cmd.Start()
}
func pauseMusic()  { exec.Command("pkill", "-STOP", "mpv").Run() }
func resumeMusic() { exec.Command("pkill", "-CONT", "mpv").Run() }
func stopMusic()   { exec.Command("pkill", "mpv").Run() }

func loadConfig() Config {
	data, err := os.ReadFile(CONFIG_FILE)
	if err != nil {
		fmt.Println(ColorRed + "[WARNING] Creating default config.json. Please edit it with your full syllabus." + ColorReset)

		defaultConfig := Config{
			SyllabusEndDate:          time.Now().AddDate(0, 3, 0).Format(TIME_FORMAT),
			ExamDate:                 time.Now().AddDate(0, 3, 10).Format(TIME_FORMAT),
			DailyStudyHrs:            8.0,
			MaxSessionHrs:            1.5,
			DailyBufferMins:          30,
			WeeklyRestDay:            time.Sunday,
			RestDayActivity:          "Mock Test & Review",
			InitialDifficultyRating:  3.0,
			DifficultyAdjustmentRate: 0.1,
			InitialWorkload: []ChapterWorkload{

    {ID: "PH001", Subject: "Physics", Chapter: "Motion in a Straight Line", InitialTotalTime: 8.5, Weightage: 1.2, InitialRevisionIntervalDays: 3, Difficulty: 3.0, RemainingTime: 8.5, IsStudyCompleted: false},
    {ID: "PH002", Subject: "Physics", Chapter: "Laws of Motion", InitialTotalTime: 15.0, Weightage: 2.0, InitialRevisionIntervalDays: 2, Difficulty: 4.0, RemainingTime: 15.0, IsStudyCompleted: false},
    {ID: "PH003", Subject: "Physics", Chapter: "Motion in a Plane", InitialTotalTime: 12.0, Weightage: 1.8, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 12.0, IsStudyCompleted: false},
    {ID: "PH004", Subject: "Physics", Chapter: "Work, Energy and Power", InitialTotalTime: 15.0, Weightage: 2.0, InitialRevisionIntervalDays: 3, Difficulty: 3.5, RemainingTime: 15.0, IsStudyCompleted: false},
    {ID: "PH005", Subject: "Physics", Chapter: "System of Particles and Rotational Motion", InitialTotalTime: 12.0, Weightage: 2.0, InitialRevisionIntervalDays: 3, Difficulty: 4.0, RemainingTime: 12.0, IsStudyCompleted: false},
    {ID: "PH006", Subject: "Physics", Chapter: "Gravitation", InitialTotalTime: 10.0, Weightage: 2.0, InitialRevisionIntervalDays: 4, Difficulty: 3.5, RemainingTime: 10.0, IsStudyCompleted: false},
    {ID: "PH007", Subject: "Physics", Chapter: "Mechanical Properties of Solids", InitialTotalTime: 8.0, Weightage: 1.2, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 8.0, IsStudyCompleted: false},
    {ID: "PH008", Subject: "Physics", Chapter: "Mechanical Properties of Fluids", InitialTotalTime: 8.0, Weightage: 1.2, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 8.0, IsStudyCompleted: false},
    {ID: "PH009", Subject: "Physics", Chapter: "Thermal Properties of Matter", InitialTotalTime: 8.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 3.0, RemainingTime: 8.0, IsStudyCompleted: false},
    {ID: "PH010", Subject: "Physics", Chapter: "Thermodynamics", InitialTotalTime: 10.0, Weightage: 1.5, InitialRevisionIntervalDays: 4, Difficulty: 3.5, RemainingTime: 10.0, IsStudyCompleted: false},
    {ID: "PH011", Subject: "Physics", Chapter: "Kinetic Theory of Gases", InitialTotalTime: 8.0, Weightage: 1.2, InitialRevisionIntervalDays: 5, Difficulty: 3.0, RemainingTime: 8.0, IsStudyCompleted: false},
    {ID: "PH012", Subject: "Physics", Chapter: "Oscillations", InitialTotalTime: 8.0, Weightage: 1.2, InitialRevisionIntervalDays: 5, Difficulty: 3.0, RemainingTime: 8.0, IsStudyCompleted: false},
    {ID: "PH013", Subject: "Physics", Chapter: "Waves", InitialTotalTime: 8.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 3.0, RemainingTime: 8.0, IsStudyCompleted: false},
    {ID: "PH014", Subject: "Physics", Chapter: "Electrostatics", InitialTotalTime: 12.0, Weightage: 2.0, InitialRevisionIntervalDays: 3, Difficulty: 4.0, RemainingTime: 12.0, IsStudyCompleted: false},
    {ID: "PH015", Subject: "Physics", Chapter: "Current Electricity", InitialTotalTime: 12.0, Weightage: 2.0, InitialRevisionIntervalDays: 3, Difficulty: 4.0, RemainingTime: 12.0, IsStudyCompleted: false},
    {ID: "PH016", Subject: "Physics", Chapter: "Magnetic Effects of Current and Magnetism", InitialTotalTime: 15.0, Weightage: 2.5, InitialRevisionIntervalDays: 2, Difficulty: 4.0, RemainingTime: 15.0, IsStudyCompleted: false},
    {ID: "PH017", Subject: "Physics", Chapter: "Electromagnetic Induction and Alternating Currents", InitialTotalTime: 18.0, Weightage: 2.5, InitialRevisionIntervalDays: 2, Difficulty: 4.5, RemainingTime: 18.0, IsStudyCompleted: false},
    {ID: "PH018", Subject: "Physics", Chapter: "Electromagnetic Waves", InitialTotalTime: 10.0, Weightage: 1.4, InitialRevisionIntervalDays: 4, Difficulty: 3.5, RemainingTime: 10.0, IsStudyCompleted: false},
    {ID: "PH019", Subject: "Physics", Chapter: "Ray Optics", InitialTotalTime: 8.0, Weightage: 1.6, InitialRevisionIntervalDays: 5, Difficulty: 3.0, RemainingTime: 8.0, IsStudyCompleted: false},
    {ID: "PH020", Subject: "Physics", Chapter: "Wave Optics", InitialTotalTime: 8.0, Weightage: 1.2, InitialRevisionIntervalDays: 5, Difficulty: 3.0, RemainingTime: 8.0, IsStudyCompleted: false},
    {ID: "PH021", Subject: "Physics", Chapter: "Dual Nature of Matter and Radiation", InitialTotalTime: 10.0, Weightage: 1.4, InitialRevisionIntervalDays: 4, Difficulty: 2.5, RemainingTime: 10.0, IsStudyCompleted: false},
    {ID: "PH022", Subject: "Physics", Chapter: "Atoms and Nuclei", InitialTotalTime: 12.0, Weightage: 2.0, InitialRevisionIntervalDays: 3, Difficulty: 3.5, RemainingTime: 12.0, IsStudyCompleted: false},
    {ID: "PH023", Subject: "Physics", Chapter: "Electronic Devices", InitialTotalTime: 8.0, Weightage: 2.0, InitialRevisionIntervalDays: 5, Difficulty: 3.0, RemainingTime: 8.0, IsStudyCompleted: false},

    {ID: "CH001", Subject: "Chemistry", Chapter: "Some Basic Concepts of Chemistry", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
    {ID: "CH002", Subject: "Chemistry", Chapter: "Structure of Atom", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 3.0, RemainingTime: 6.0, IsStudyCompleted: false},
    {ID: "CH003", Subject: "Chemistry", Chapter: "Classification of Elements and Periodicity in Properties", InitialTotalTime: 7.0, Weightage: 1.2, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 7.0, IsStudyCompleted: false},
    {ID: "CH004", Subject: "Chemistry", Chapter: "Chemical Bonding and Molecular Structure", InitialTotalTime: 11.0, Weightage: 1.8, InitialRevisionIntervalDays: 3, Difficulty: 4.5, RemainingTime: 11.0, IsStudyCompleted: false},
    {ID: "CH005", Subject: "Chemistry", Chapter: "States of Matter: Gases and Liquids", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 3.0, RemainingTime: 6.0, IsStudyCompleted: false},
    {ID: "CH006", Subject: "Chemistry", Chapter: "Thermodynamics", InitialTotalTime: 8.0, Weightage: 1.4, InitialRevisionIntervalDays: 4, Difficulty: 3.5, RemainingTime: 8.0, IsStudyCompleted: false},
    {ID: "CH007", Subject: "Chemistry", Chapter: "Equilibrium", InitialTotalTime: 8.0, Weightage: 1.4, InitialRevisionIntervalDays: 4, Difficulty: 3.5, RemainingTime: 8.0, IsStudyCompleted: false},
    {ID: "CH008", Subject: "Chemistry", Chapter: "Redox Reactions", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 3.5, RemainingTime: 6.0, IsStudyCompleted: false},
    {ID: "CH009", Subject: "Chemistry", Chapter: "Hydrogen", InitialTotalTime: 5.0, Weightage: 0.8, InitialRevisionIntervalDays: 6, Difficulty: 2.0, RemainingTime: 5.0, IsStudyCompleted: false},
    {ID: "CH010", Subject: "Chemistry", Chapter: "s-Block Elements (Alkali and Alkaline earth metals)", InitialTotalTime: 7.0, Weightage: 1.2, InitialRevisionIntervalDays: 5, Difficulty: 3.0, RemainingTime: 7.0, IsStudyCompleted: false},
    {ID: "CH011", Subject: "Chemistry", Chapter: "p-Block Elements", InitialTotalTime: 9.0, Weightage: 1.5, InitialRevisionIntervalDays: 3, Difficulty: 3.5, RemainingTime: 9.0, IsStudyCompleted: false},
    {ID: "CH012", Subject: "Chemistry", Chapter: "Organic Chemistry - Some Basic Principles and Techniques", InitialTotalTime: 9.0, Weightage: 1.5, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 9.0, IsStudyCompleted: false},
    {ID: "CH013", Subject: "Chemistry", Chapter: "Hydrocarbons", InitialTotalTime: 11.0, Weightage: 1.8, InitialRevisionIntervalDays: 3, Difficulty: 3.5, RemainingTime: 11.0, IsStudyCompleted: false},
    {ID: "CH014", Subject: "Chemistry", Chapter: "Biomolecules", InitialTotalTime: 9.0, Weightage: 1.5, InitialRevisionIntervalDays: 3, Difficulty: 2.5, RemainingTime: 9.0, IsStudyCompleted: false},
    {ID: "CH015", Subject: "Chemistry", Chapter: "Polymers", InitialTotalTime: 5.0, Weightage: 0.8, InitialRevisionIntervalDays: 6, Difficulty: 3.0, RemainingTime: 5.0, IsStudyCompleted: false},
    {ID: "CH016", Subject: "Chemistry", Chapter: "Chemistry in Everyday Life", InitialTotalTime: 5.0, Weightage: 0.8, InitialRevisionIntervalDays: 6, Difficulty: 2.5, RemainingTime: 5.0, IsStudyCompleted: false},

        {ID: "BI001", Subject: "Biology", Chapter: "The Living World", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.0, RemainingTime: 6.0, IsStudyCompleted: false},
        {ID: "BI002", Subject: "Biology", Chapter: "Biological Classification", InitialTotalTime: 7.0, Weightage: 1.2, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 7.0, IsStudyCompleted: false},
        {ID: "BI003", Subject: "Biology", Chapter: "Plant Kingdom", InitialTotalTime: 6.5, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.5, IsStudyCompleted: false},
        {ID: "BI004", Subject: "Biology", Chapter: "Animal Kingdom", InitialTotalTime: 8.0, Weightage: 1.5, InitialRevisionIntervalDays: 4, Difficulty: 3.5, RemainingTime: 8.0, IsStudyCompleted: false},
        {ID: "BI005", Subject: "Biology", Chapter: "Morphology of Flowering Plants", InitialTotalTime: 5.5, Weightage: 0.8, InitialRevisionIntervalDays: 6, Difficulty: 2.0, RemainingTime: 5.5, IsStudyCompleted: false},
        {ID: "BI006", Subject: "Biology", Chapter: "Anatomy of Flowering Plants", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
        {ID: "BI007", Subject: "Biology", Chapter: "Structural Organisation in Animals", InitialTotalTime: 6.5, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.5, IsStudyCompleted: false},
        {ID: "BI008", Subject: "Biology", Chapter: "Cell Structure and Function", InitialTotalTime: 8.0, Weightage: 1.2, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 8.0, IsStudyCompleted: false},
        {ID: "BI009", Subject: "Biology", Chapter: "Biomolecules", InitialTotalTime: 9.0, Weightage: 1.5, InitialRevisionIntervalDays: 3, Difficulty: 3.0, RemainingTime: 9.0, IsStudyCompleted: false},
        {ID: "BI010", Subject: "Biology", Chapter: "Cell Cycle and Cell Division", InitialTotalTime: 7.5, Weightage: 1.2, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 7.5, IsStudyCompleted: false},
        {ID: "BI011", Subject: "Biology", Chapter: "Transport in Plants", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
        {ID: "BI012", Subject: "Biology", Chapter: "Mineral Nutrition", InitialTotalTime: 5.0, Weightage: 0.8, InitialRevisionIntervalDays: 6, Difficulty: 2.0, RemainingTime: 5.0, IsStudyCompleted: false},
        {ID: "BI013", Subject: "Biology", Chapter: "Photosynthesis in Higher Plants", InitialTotalTime: 7.0, Weightage: 1.2, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 7.0, IsStudyCompleted: false},
        {ID: "BI014", Subject: "Biology", Chapter: "Respiration in Plants", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
        {ID: "BI015", Subject: "Biology", Chapter: "Plant Growth and Development", InitialTotalTime: 6.5, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.5, IsStudyCompleted: false},
        {ID: "BI016", Subject: "Biology", Chapter: "Digestion and Absorption", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
        {ID: "BI017", Subject: "Biology", Chapter: "Breathing and Exchange of Gases", InitialTotalTime: 6.5, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.5, IsStudyCompleted: false},
        {ID: "BI018", Subject: "Biology", Chapter: "Body Fluids and Circulation", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
        {ID: "BI019", Subject: "Biology", Chapter: "Excretory Products and Their Elimination", InitialTotalTime: 5.0, Weightage: 0.8, InitialRevisionIntervalDays: 6, Difficulty: 2.0, RemainingTime: 5.0, IsStudyCompleted: false},
        {ID: "BI020", Subject: "Biology", Chapter: "Locomotion and Movement", InitialTotalTime: 6.5, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.5, IsStudyCompleted: false},
        {ID: "BI021", Subject: "Biology", Chapter: "Neural Control and Coordination", InitialTotalTime: 7.0, Weightage: 1.2, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 7.0, IsStudyCompleted: false},
        {ID: "BI022", Subject: "Biology", Chapter: "Chemical Coordination and Integration", InitialTotalTime: 6.5, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.5, IsStudyCompleted: false},

        {ID: "BI023", Subject: "Biology", Chapter: "Reproduction in Organisms", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
        {ID: "BI024", Subject: "Biology", Chapter: "Sexual Reproduction in Flowering Plants", InitialTotalTime: 7.0, Weightage: 1.2, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 7.0, IsStudyCompleted: false},
        {ID: "BI025", Subject: "Biology", Chapter: "Human Reproduction", InitialTotalTime: 8.0, Weightage: 1.5, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 8.0, IsStudyCompleted: false},
        {ID: "BI026", Subject: "Biology", Chapter: "Reproductive Health", InitialTotalTime: 5.0, Weightage: 0.8, InitialRevisionIntervalDays: 6, Difficulty: 2.0, RemainingTime: 5.0, IsStudyCompleted: false},
        {ID: "BI027", Subject: "Biology", Chapter: "Principles of Inheritance and Variation", InitialTotalTime: 8.0, Weightage: 1.5, InitialRevisionIntervalDays: 4, Difficulty: 3.5, RemainingTime: 8.0, IsStudyCompleted: false},
        {ID: "BI028", Subject: "Biology", Chapter: "Molecular Basis of Inheritance", InitialTotalTime: 9.0, Weightage: 1.8, InitialRevisionIntervalDays: 3, Difficulty: 4.0, RemainingTime: 9.0, IsStudyCompleted: false},
        {ID: "BI029", Subject: "Biology", Chapter: "Evolution", InitialTotalTime: 6.5, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.5, IsStudyCompleted: false},
        {ID: "BI030", Subject: "Biology", Chapter: "Human Health and Disease", InitialTotalTime: 6.5, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.5, IsStudyCompleted: false},
        {ID: "BI031", Subject: "Biology", Chapter: "Strategies for Enhancement in Food Production", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
        {ID: "BI032", Subject: "Biology", Chapter: "Microbes in Human Welfare", InitialTotalTime: 5.5, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.0, RemainingTime: 5.5, IsStudyCompleted: false},
        {ID: "BI033", Subject: "Biology", Chapter: "Biotechnology: Principles and Processes", InitialTotalTime: 7.5, Weightage: 1.5, InitialRevisionIntervalDays: 4, Difficulty: 3.5, RemainingTime: 7.5, IsStudyCompleted: false},
        {ID: "BI034", Subject: "Biology", Chapter: "Biotechnology and Its Applications", InitialTotalTime: 7.0, Weightage: 1.2, InitialRevisionIntervalDays: 4, Difficulty: 3.0, RemainingTime: 7.0, IsStudyCompleted: false},
        {ID: "BI035", Subject: "Biology", Chapter: "Organisms and Populations", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
        {ID: "BI036", Subject: "Biology", Chapter: "Ecosystem", InitialTotalTime: 6.5, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.5, IsStudyCompleted: false},
        {ID: "BI037", Subject: "Biology", Chapter: "Biodiversity and Conservation", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
        {ID: "BI038", Subject: "Biology", Chapter: "Environmental Issues", InitialTotalTime: 6.0, Weightage: 1.0, InitialRevisionIntervalDays: 5, Difficulty: 2.5, RemainingTime: 6.0, IsStudyCompleted: false},
    },
			}
		saveConfig(defaultConfig)
		return defaultConfig
	}
	var config Config
	json.Unmarshal(data, &config)
	return config
}

func saveConfig(c Config) {
	data, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(CONFIG_FILE, data, 0644)
}

func loadState() (ScheduleState, bool) {
	data, err := os.ReadFile(STATE_FILE)
	if err != nil {
		fmt.Println(ColorYellow + "[INIT] State file not found. Initializing ScheduleState from config." + ColorReset)
		return initializeState(loadConfig()), false 
	}
	var state ScheduleState
	if err := json.Unmarshal(data, &state); err != nil {
		fmt.Println(ColorRed + "[ERROR] Failed to unmarshal state file. Re-initializing." + ColorReset)
		return initializeState(loadConfig()), false 
	}
	return state, true 
}

func initializeState(c Config) ScheduleState {
	state := ScheduleState{
		Workload:          make(map[string]ChapterWorkload),
		LastScheduledDate: time.Now().AddDate(0, 0, -1).Format(TIME_FORMAT),
	}
	for i, wl := range c.InitialWorkload {

		if wl.ID == "" {
			wl.ID = fmt.Sprintf("C%03d", i+1)
		}
		wl.RemainingTime = wl.InitialTotalTime
		wl.Difficulty = c.InitialDifficultyRating
		state.Workload[wl.ID] = wl
	}
	return state
}

func saveState(s ScheduleState) {
	data, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(STATE_FILE, data, 0644)
}

func deleteScheduleState() {
	os.Remove(STATE_FILE)
}

func dayPlanFilePath(date time.Time) string {
	return filepath.Join(SCHEDULE_DIR, date.Format(TIME_FORMAT)+".txt")
}

func writeDayPlan(date time.Time, sessions []Session) {
	if err := os.MkdirAll(SCHEDULE_DIR, os.ModePerm); err != nil {
		fmt.Printf(ColorRed+"[CRITICAL ERROR] Failed to create directory '%s': %v\n"+ColorReset, SCHEDULE_DIR, err)
		return
	}
	filepath := dayPlanFilePath(date)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DATE: %s (%s)\n\n", date.Format(TIME_FORMAT), date.Weekday()))
	for i, session := range sessions {
		header := fmt.Sprintf("SESSION %d:", i+1)
		if session.Type == "Buffer" || session.Type == "Rest" {
			header = strings.ToUpper(session.Type) + ":"
		}
		sb.WriteString(fmt.Sprintf("%s\n", header))
		sb.WriteString(fmt.Sprintf("  Subject:  %s\n", session.Subject))
		sb.WriteString(fmt.Sprintf("  Chapter:  %s\n", session.Chapter))
		sb.WriteString(fmt.Sprintf("  Duration: %.1f hrs\n", session.Duration))
		sb.WriteString(fmt.Sprintf("  Status:   %s\n", session.Status))
		sb.WriteString(fmt.Sprintf("  Type:     %s\n", session.Type))
		if session.ChapterID != "" {
			sb.WriteString(fmt.Sprintf("  ID:       %s\n", session.ChapterID))
		}
		sb.WriteString("\n")
	}
	os.WriteFile(filepath, []byte(sb.String()), 0644)
}

func readDayPlan(date time.Time) ([]Session, error) {
	filepath := dayPlanFilePath(date)
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
		if (session.Subject != "" && session.Duration > 0) || session.Type == "Rest" || session.Type == "Buffer" {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

func loadProgress(today time.Time) (Progress, bool) {
	data, err := os.ReadFile(PROGRESS_FILE)
	if err != nil {
		return Progress{}, false
	}
	var p Progress
	if err := json.Unmarshal(data, &p); err != nil {
		deleteProgress()
		return Progress{}, false
	}

	if p.Date != today.Format(TIME_FORMAT) {
		deleteProgress()
		return Progress{}, false
	}
	return p, true
}

func saveProgress(chapterID string, elapsed int) {
	p := Progress{
		ChapterID:      chapterID,
		ElapsedSeconds: elapsed,
		Date:           time.Now().Truncate(24 * time.Hour).Format(TIME_FORMAT),
	}
	data, _ := json.MarshalIndent(p, "", "  ")

	os.WriteFile(PROGRESS_FILE, data, 0644) 
}

func deleteProgress() {
	os.Remove(PROGRESS_FILE)
}

func updateChapterPerformance(wl ChapterWorkload, success bool) ChapterWorkload {
	rate := rawConfig.DifficultyAdjustmentRate
	if success {

		wl.Difficulty = math.Max(1.0, wl.Difficulty-rate)
	} else {

		wl.Difficulty = math.Min(5.0, wl.Difficulty+rate*2.0)
	}
	return wl
}

func calculateWeightedTime(wl ChapterWorkload) float64 {

	return wl.RemainingTime * (1 + wl.Difficulty/5.0) * (wl.Weightage * 2.0)
}

func calculateQuotas(state *ScheduleState) []ChapterWorkload {
	today := time.Now().Truncate(24 * time.Hour)
	syllabusEndDate, _ := time.Parse(TIME_FORMAT, rawConfig.SyllabusEndDate)
	totalWorkload := 0.0
	totalRemainingTime := 0.0
	netStudyDays := 0
	var allChapters []ChapterWorkload
	for id, wl := range state.Workload {
		if !wl.IsStudyCompleted && wl.RemainingTime > 0.001 {
			wl.PriorityScore = calculateWeightedTime(wl)
			totalWorkload += wl.PriorityScore
			totalRemainingTime += wl.RemainingTime
		} else if wl.RevisionCount < MAX_REVISIONS && wl.NextRevisionDate != "" {
			revDate, _ := time.Parse(TIME_FORMAT, wl.NextRevisionDate)
			daysUntilDue := revDate.Sub(today).Hours() / 24.0

			wl.PriorityScore = (wl.Difficulty / 5.0) * wl.Weightage * (10.0 / math.Max(1.0, daysUntilDue))
			totalWorkload += wl.PriorityScore * REVISION_TIME_HRS
		} else {
			wl.PriorityScore = 0.0
		}
		state.Workload[id] = wl
		allChapters = append(allChapters, wl)
	}
	currentDate := today
	if state.LastScheduledDate != "" {
		stateDate, _ := time.Parse(TIME_FORMAT, state.LastScheduledDate)
		if stateDate.After(today) {
			currentDate = stateDate
		}
	}
	for currentDate.Before(syllabusEndDate.AddDate(0, 0, 1)) {
		if currentDate.Weekday() != rawConfig.WeeklyRestDay {
			netStudyDays++
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}
	dailyQuotaWT := 0.0
	if netStudyDays > 0 {
		dailyQuotaWT = totalWorkload / float64(netStudyDays)
	}
	state.TotalWeightedWorkload = totalWorkload
	state.TotalRemainingTime = totalRemainingTime
	state.NetStudyDays = netStudyDays
	state.DailyQuotaWT = dailyQuotaWT
	return allChapters
}

func prioritizeChapters(allChapters []ChapterWorkload) []ChapterWorkload {
	var activeStudyChapters []ChapterWorkload
	for _, wl := range allChapters {
		if !wl.IsStudyCompleted && wl.RemainingTime > 0.001 {
			activeStudyChapters = append(activeStudyChapters, wl)
		}
	}

	sort.Slice(activeStudyChapters, func(i, j int) bool {
		return activeStudyChapters[i].PriorityScore > activeStudyChapters[j].PriorityScore
	})
	return activeStudyChapters
}

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

func generateSchedule() {
	fmt.Println("--- Starting Schedule Generation ---")
	state, _ := loadState()
	realToday := time.Now().Truncate(24 * time.Hour)
	stateDate, _ := time.Parse(TIME_FORMAT, state.LastScheduledDate)
	syllabusEndDate, _ := time.Parse(TIME_FORMAT, rawConfig.SyllabusEndDate)

	if stateDate.Before(realToday) {
		state.LastScheduledDate = realToday.Format(TIME_FORMAT)
		saveState(state)
		fmt.Printf("[FIX] Schedule path reset detected. Starting generation from today: %s\n", realToday.Format(TIME_FORMAT))
	}

	allChapters := calculateQuotas(&state)
	allChapters = prioritizeChapters(allChapters)
	currentDate, _ := time.Parse(TIME_FORMAT, state.LastScheduledDate)

	if state.TotalRemainingTime <= 0.001 && len(getDueRevisions(state, currentDate)) == 0 {
		if currentDate.After(syllabusEndDate) {
			fmt.Println("[SUCCESS] All chapters are studied and all revisions are up-to-date. No new schedule generated.")
			return
		}
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
				Subject: "Rest", Chapter: rawConfig.RestDayActivity, Duration: rawConfig.DailyStudyHrs, Type: "Rest", Status: "Pending",
			})
		} else {

			dueRevisions := getDueRevisions(state, currentDate)
			sort.Slice(dueRevisions, func(i, j int) bool { return dueRevisions[i].PriorityScore > dueRevisions[j].PriorityScore })
			for len(dueRevisions) > 0 && hoursAssigned < dailyTotalStudyHrs {
				revChapter := dueRevisions[0]
				revDuration := math.Min(REVISION_TIME_HRS, dailyTotalStudyHrs-hoursAssigned)
				if revDuration <= 0.001 {
					break
				}
				dailySessions = append(dailySessions, Session{
					Subject: revChapter.Subject, Chapter: fmt.Sprintf("%s (Revision #%d)", revChapter.Chapter, revChapter.RevisionCount+1), Duration: revDuration, ChapterID: revChapter.ID, Type: "Revision", Status: "Pending",
				})
				hoursAssigned += revDuration
				lastSubject = revChapter.Subject
				dueRevisions = dueRevisions[1:]
			}

			var currentActive []*ChapterWorkload
			for _, ch := range activeStudyChapters {
				if !ch.IsStudyCompleted && ch.RemainingTime > 0.001 {
					currentActive = append(currentActive, ch)
				}
			}
			activeStudyChapters = currentActive
			sort.Slice(activeStudyChapters, func(i, j int) bool { return activeStudyChapters[i].PriorityScore > activeStudyChapters[j].PriorityScore })

			for dailyProgressWT < state.DailyQuotaWT && hoursAssigned < dailyTotalStudyHrs && len(activeStudyChapters) > 0 {
				foundChapterIndex := -1

				for i, ch := range activeStudyChapters {
					if ch.Subject != lastSubject {
						foundChapterIndex = i
						break
					}
				}
				if foundChapterIndex == -1 {

					foundChapterIndex = 0
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
					Subject: currentChapter.Subject, Chapter: currentChapter.Chapter, Duration: sessionDuration, ChapterID: currentChapter.ID, Type: "Study", Status: "Pending",
				})

				dailyProgressWT += sessionWT
				hoursAssigned += sessionDuration
				lastSubject = currentChapter.Subject
				currentChapter.RemainingTime -= sessionDuration

				if currentChapter.RemainingTime <= 0.001 {
					currentChapter.IsStudyCompleted = true

					activeStudyChapters = append(activeStudyChapters[:foundChapterIndex], activeStudyChapters[foundChapterIndex+1:]...)

					currentChapter.NextRevisionDate = currentDate.AddDate(0, 0, currentChapter.InitialRevisionIntervalDays).Format(TIME_FORMAT)
				}
			}

			dailySessions = append(dailySessions, Session{
				Subject: "Buffer", Chapter: "Recovery/Review", Duration: float64(rawConfig.DailyBufferMins) / 60.0, Type: "Buffer", Status: "Pending",
			})
		}

		for _, chPtr := range activeStudyChapters {
			state.Workload[chPtr.ID] = *chPtr
		}

		writeDayPlan(currentDate, dailySessions)

		currentDate = currentDate.AddDate(0, 0, 1)
		state.LastScheduledDate = currentDate.Format(TIME_FORMAT)
	}

	saveState(state)
	fmt.Println("\n--- Schedule Generation Complete ---")
	fmt.Printf("Syllabus plans saved in the '%s/' directory until %s.\n", SCHEDULE_DIR, syllabusEndDate.Format(TIME_FORMAT))
}

func processMissedSessionsForDate(date time.Time) ([]Session, error) {
	sessions, err := readDayPlan(date)
	if err != nil {
		return nil, err
	}
	missedSessions := []Session{}
	updated := false
	modifiedSessions := make([]Session, len(sessions))
	copy(modifiedSessions, sessions)
	for i, s := range modifiedSessions {
		if s.Status == "Pending" && (s.Type == "Study" || s.Type == "Revision") {
			modifiedSessions[i].Status = "Missed"
			missedSessions = append(missedSessions, modifiedSessions[i])
			updated = true
		}
	}
	if updated {
		writeDayPlan(date, modifiedSessions)
	}
	return missedSessions, nil
}

func adjustWorkload(missedSessions []Session, auditDate time.Time) {
	fmt.Println("\n[ADJUSTMENT] Recalculating workload due to missed sessions...")
	state, _ := loadState()
	if len(state.Workload) == 0 {
		fmt.Println("[WARNING] No active workload in state. Skipping adjustment.")
		return
	}
	for _, session := range missedSessions {
		chID := session.ChapterID
		duration := session.Duration
		if chID != "" {
			if workload, ok := state.Workload[chID]; ok {

				workload = updateChapterPerformance(workload, false) 
				if session.Type == "Revision" {

					workload.NextRevisionDate = auditDate.AddDate(0, 0, 1).Format(TIME_FORMAT)
					workload.RevisionCount--
					workload.RevisionCount = int(math.Max(0, float64(workload.RevisionCount)))
					fmt.Printf("  -> Missed Revision for %s. Resetting due date.\n", workload.Chapter)
				} else {

					workload.RemainingTime += duration
					fmt.Printf("  -> Added %.1f hrs back to initial study of %s.\n", duration, workload.Chapter)
				}
				state.Workload[chID] = workload
			}
		}
	}

	restartDate := auditDate.AddDate(0, 0, 1)
	state.LastScheduledDate = restartDate.Format(TIME_FORMAT)
	saveState(state)
	fmt.Printf("[ADJUSTMENT] Re-generating schedule from %s with adjusted workload...\n", restartDate.Format(TIME_FORMAT))
	generateSchedule()
	fmt.Println("[ADJUSTMENT] Schedule successfully updated and re-balanced.")
}

func inputReader(cmdChan chan<- command) {
	reader := bufio.NewReader(os.Stdin)
	for {
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "" {
			cmdChan <- command{action: input}
		}
	}
}

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
		remaining := totalSeconds - elapsedSeconds
		fmt.Printf("\033[2K\r[TIMER] Remaining: %s | Status: RUNNING  ",time.Duration(remaining)*time.Second)
	}

	musicOn := true
	startMusic()

	paused := false
	missedSessions := []Session{}
	ticker := time.NewTicker(time.Second)
	saveTicker := time.NewTicker(PROGRESS_SAVE_INTERVAL) 
	stopTimerChan := make(chan bool)
	cmdChan := make(chan command)
	go inputReader(cmdChan)

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
	
	fmt.Printf("[Timer] %s\n", session.Chapter)
	finished := false
	for elapsedSeconds < totalSeconds && !finished {
		select {
		case cmd := <-cmdChan:
			switch cmd.action {
				case "o" :
				if musicOn {
					stopMusic() 
					musicOn = false
					fmt.Println("\n[ACTION] Music OFF. Press 'o' to turn music back on. (Timer continues)")
				}else {
					startMusic()
					musicOn = true 
					fmt.Println("\n[ACTION] Music On. Press 'o' to turn music back off. (Timer continues)")
				}
			case "p":
				if !paused {
					if musicOn {
					pauseMusic()
					}
					paused = true
					fmt.Print("\n[ACTION] Paused. Enter 'r' to resume, 'f' to finish early, or 'm' to mark missed. ")
					if session.ChapterID != "" {
						saveProgress(session.ChapterID, elapsedSeconds)
					}
				}
			case "r":
				if paused {
					if musicOn {
					resumeMusic()
					}
					paused = false
					startTime = time.Now().Add(time.Duration(-elapsedSeconds) * time.Second)
					remaining := totalSeconds - elapsedSeconds
					fmt.Printf("\033[2K\r[TIMER] Remaining: %s | Status: RUNNING  ",time.Duration(remaining)*time.Second)
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
				if paused {
					fmt.Print("Invalid command. Options: p, r, f, m. ")
				}
			}
			case <-ticker.C:
				if !paused {
					elapsedSeconds = int(time.Since(startTime).Seconds())
				}
				remaining := totalSeconds - elapsedSeconds
					status := "RUNNING"
					if paused {
						status = "PAUSED"
					}
					fmt.Printf("\033[2K\r[TIMER] Remaining: %s | Status: %s", time.Duration(remaining)*time.Second, status)
				if remaining <= 0 {
					finished = true
				}
		}
	}

	close(stopTimerChan)
	ticker.Stop()
	if musicOn {
	stopMusic()
	}

	if session.Status != "Missed" {
		session.Status = "Completed"
		if elapsedSeconds >= totalSeconds {
			fmt.Println("\n\n" + ColorGreen + "[COMPLETED] Session finished! Great job. 🔔" + ColorReset)
		}

		if session.ChapterID != "" {
			state, _ := loadState()
			if workload, ok := state.Workload[session.ChapterID]; ok {
				workload = updateChapterPerformance(workload, true)
				if session.Type == "Revision" {
					workload.RevisionCount++
					if workload.RevisionCount < MAX_REVISIONS {

						nextInterval := workload.InitialRevisionIntervalDays * (workload.RevisionCount + 1)
						workload.NextRevisionDate = today.AddDate(0, 0, nextInterval).Format(TIME_FORMAT)
					} else {
						workload.NextRevisionDate = "" 
					}
				} else {

					timeSpent := session.Duration
					if elapsedSeconds < totalSeconds {
						timeSpent = float64(elapsedSeconds) / 3600.0
					}
					workload.RemainingTime = math.Max(0, workload.RemainingTime-timeSpent)

					if workload.RemainingTime <= 0.001 {
						workload.IsStudyCompleted = true

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

	deleteProgress()
	writeDayPlan(today, sessions)
	adjustWorkload(missedSessions, today)
	return true, sessions
}

func runBreakTimer(durationMins int) {
	totalSeconds := durationMins * 60
	elapsedSeconds := 0
	startTime := time.Now()
	paused := false
	ticker := time.NewTicker(time.Second)
	cmdChan := make(chan command)
	go inputReader(cmdChan)
	fmt.Printf("\n" + ColorCyan + "[BREAK] Starting %d minute break. Press 'q' to skip, 'p' to pause. ☕️" + ColorReset + "\n", durationMins)
	for elapsedSeconds < totalSeconds {
		select {
		case cmd := <-cmdChan:
			switch cmd.action {
			case "q":
				fmt.Println("\n[ACTION] Break skipped.")
				elapsedSeconds = totalSeconds
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
			if elapsedSeconds%1 == 0 || remaining <= 5 {
				status := "RUNNING"
				if paused {
					status = "PAUSED"
				}
				fmt.Printf("\033[2K\r[TIMER] Break Remaining: %s | Status: %s ", time.Duration(remaining)*time.Second, status)
			}
			if remaining <= 0 {
			}
		}
	}
	ticker.Stop()
	if elapsedSeconds >= totalSeconds {
		fmt.Println("\n\n" + ColorGreen + "[BREAK] Break finished! Time to select your next session." + ColorReset)
	}
}

func runTimerCLI() {
	rawConfig = loadConfig()
	realToday := time.Now().Truncate(24 * time.Hour)
	fmt.Printf("\n--- Timer CLI for %s ---\n", realToday.Format(TIME_FORMAT))
	state, _ := loadState()

	lastScheduled, _ := time.Parse(TIME_FORMAT, state.LastScheduledDate)
	missedSessionsAcrossDays := []Session{}

	for d := lastScheduled; d.Before(realToday); d = d.AddDate(0, 0, 1) {
		missed, err := processMissedSessionsForDate(d)
		if err == nil && len(missed) > 0 {
			fmt.Printf("[AUDIT] Found %d missed sessions on %s. Adjusting workload.\n", len(missed), d.Format(TIME_FORMAT))
			missedSessionsAcrossDays = append(missedSessionsAcrossDays, missed...)
		}
	}

	if len(missedSessionsAcrossDays) > 0 {
		fmt.Printf("[RE-BALANCING] Total %d missed sessions detected. Adjusting workload and regenerating path from TODAY (%s)...\n", len(missedSessionsAcrossDays), realToday.Format(TIME_FORMAT))
		adjustWorkload(missedSessionsAcrossDays, realToday.AddDate(0, 0, -1))
	} else if lastScheduled.Before(realToday.AddDate(0, 0, 1)) {
		fmt.Println("[RE-BALANCING] Schedule is behind. Regenerating path to ensure today is planned.")
		state.LastScheduledDate = realToday.Format(TIME_FORMAT)
		saveState(state)
		generateSchedule()
	}

	sessions, err := readDayPlan(realToday)
	if err != nil {
		fmt.Printf("[ERROR] Could not load today's schedule. Run '3' (RE-GENERATE) first: %v\n", err)
		return
	}

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
			fmt.Printf("\n" + ColorYellow + "[RESUME ALERT] Unfinished session found for %s - %s (%s elapsed)." + ColorReset + "\n",
				sessionToResume.Subject, sessionToResume.Chapter, time.Duration(progress.ElapsedSeconds)*time.Second)
			fmt.Print("Do you want to resume this session? (y/N): ")
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

				sessions, _ = readDayPlan(realToday) 
			}
		} else {
			fmt.Println("[WARNING] Progress file found but chapter ID mismatch. Deleting progress file.")
			deleteProgress()
		}
	}

	for {
		fmt.Println("\n-- Today's Schedule --")
		hasPending := false

		for i, s := range sessions {
			if s.Type == "Study" || s.Type == "Revision" {
				hasPending = hasPending || (s.Status == "Pending")
				status := s.Status
				if status == "Pending" {
					status = ColorCyan + status + ColorReset
				} else if status == "Completed" {
					status = ColorGreen + status + ColorReset
				} else if status == "Missed" {
					status = ColorRed + status + ColorReset
				}
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
			modifiedSessions := make([]Session, len(sessions))
			copy(modifiedSessions, sessions)
			for i, s := range modifiedSessions {
				if s.Status == "Pending" && (s.Type == "Study" || s.Type == "Revision") {
					modifiedSessions[i].Status = "Missed"
					missed = append(missed, modifiedSessions[i])
					missedCount++
				}
			}
			if missedCount > 0 {
				writeDayPlan(realToday, modifiedSessions)
				fmt.Printf("[ACTION] Marked %d pending study/revision sessions as MISSED. These will be rescheduled.\n", missedCount)
				adjustWorkload(missed, realToday)
				sessions, _ = readDayPlan(realToday)
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

		finished, updatedSessions := runStudyTimer(sessions, sessionIdx, 0, realToday)
		sessions = updatedSessions

		if finished && (session.Type == "Study" || session.Type == "Revision") && session.Status == "Completed" {
			runBreakTimer(BREAK_MINUTES)
		}
	}

	fmt.Println("\n[INFO] Exiting timer. Any unfinished session progress has been saved.")
}

func runFullReport() {
	rawConfig = loadConfig()
	fmt.Println("\n--- FULL PROGRESS REPORT ---")
	state, _ := loadState()

	allChapters := calculateQuotas(&state)

	totalWorkload := state.TotalWeightedWorkload
	totalRemainingHrs := state.TotalRemainingTime
	netStudyDays := state.NetStudyDays
	dailyQuota := state.DailyQuotaWT

	if len(state.Workload) == 0 {
		fmt.Println("[INFO] No workload initialized. Please run option [3] RE-GENERATE first.")
		return
	}

	if totalWorkload < 0.001 {
		today := time.Now().Truncate(24 * time.Hour)
		if len(getDueRevisions(state, today)) == 0 {
			fmt.Printf("🎯 Syllabus Target Date: %s (Net Study Days Remaining: %d)\n", rawConfig.SyllabusEndDate, netStudyDays)
			fmt.Println("⏳ Total Remaining Workload: 0.00 WT (0.0 Study Hrs)")
			fmt.Println("📅 Required Daily Quota: 0.00 WT (Weighted Time)")
			fmt.Println("-----------------------------------------------------------------")
			fmt.Println("🎉 All initial study and scheduled revisions are complete!")
			fmt.Println("-----------------------------------------------------------------")
			return
		}
	}

	fmt.Printf("🎯 Syllabus Target Date: %s (Net Study Days Remaining: %d)\n", rawConfig.SyllabusEndDate, netStudyDays)
	fmt.Printf("⏳ Total Remaining Workload: %.2f WT (%.1f Study Hrs)\n", totalWorkload, totalRemainingHrs)
	fmt.Printf("📅 Required Daily Quota: %.2f WT (Weighted Time)\n", dailyQuota)
	fmt.Println("-----------------------------------------------------------------")

	var incompleteStudyChapters, revisionDueChapters, nextRevisionChapters, completedChapters []ChapterWorkload
	today := time.Now().Truncate(24 * time.Hour)

	for _, wl := range allChapters {
		if !wl.IsStudyCompleted && wl.RemainingTime > 0.001 {
			incompleteStudyChapters = append(incompleteStudyChapters, wl)
		} else if wl.IsStudyCompleted && wl.RevisionCount < MAX_REVISIONS && wl.NextRevisionDate != "" {
			revDate, _ := time.Parse(TIME_FORMAT, wl.NextRevisionDate)
			if !revDate.After(today) {
				revisionDueChapters = append(revisionDueChapters, wl)
			} else {
				nextRevisionChapters = append(nextRevisionChapters, wl)
			}
		} else if wl.IsStudyCompleted && wl.RevisionCount >= MAX_REVISIONS {
			completedChapters = append(completedChapters, wl)
		}
	}

	sort.Slice(incompleteStudyChapters, func(i, j int) bool { return incompleteStudyChapters[i].PriorityScore > incompleteStudyChapters[j].PriorityScore })
	sort.Slice(revisionDueChapters, func(i, j int) bool { return revisionDueChapters[i].PriorityScore > revisionDueChapters[j].PriorityScore })
	sort.Slice(nextRevisionChapters, func(i, j int) bool {
		dateI, _ := time.Parse(TIME_FORMAT, nextRevisionChapters[i].NextRevisionDate)
		dateJ, _ := time.Parse(TIME_FORMAT, nextRevisionChapters[j].NextRevisionDate)
		return dateI.Before(dateJ)
	})

	fmt.Println("\n📚 PENDING INITIAL STUDY (Sorted by Priority)")
	if len(incompleteStudyChapters) == 0 {
		fmt.Println("  -> All initial study complete! Time for revision phase.")
	} else {
		for _, wl := range incompleteStudyChapters {
			fmt.Printf("  - [Prio: %.2f | %.1f hrs left] %s: %s (Diff: %.1f)\n", wl.PriorityScore, wl.RemainingTime, wl.Subject, wl.Chapter, wl.Difficulty)
		}
	}

	fmt.Println("\n🔄 REVISIONS DUE TODAY")
	if len(revisionDueChapters) == 0 {
		fmt.Println("  -> No revisions are currently due for today.")
	} else {
		for _, wl := range revisionDueChapters {
			fmt.Printf("  - [DUE | Rev #%d of %d] %s: %s (Priority: %.2f)\n", wl.RevisionCount+1, MAX_REVISIONS, wl.Subject, wl.Chapter, wl.PriorityScore)
		}
	}

	fmt.Println("\n📅 UPCOMING REVISIONS")
	if len(nextRevisionChapters) == 0 {
		fmt.Println("  -> No upcoming revisions scheduled.")
	} else {
		for i, wl := range nextRevisionChapters {
			if i >= 3 {
				break
			}
			fmt.Printf("  - [Next: %s | Rev #%d of %d] %s: %s\n", wl.NextRevisionDate, wl.RevisionCount+1, MAX_REVISIONS, wl.Subject, wl.Chapter)
		}
		if len(nextRevisionChapters) > 3 {
			fmt.Printf("  ... and %d more upcoming revisions.\n", len(nextRevisionChapters)-3)
		}
	}

	total := float64(len(allChapters))
	completed := 0.0
	for _, wl := range allChapters {
		if wl.IsStudyCompleted {
			completed++
		}
	}
	studyProgress := 100.0
	if total > 0 {
		studyProgress = (completed / total) * 100
	}

	fmt.Println("\n-----------------------------------------------------------------")
	fmt.Printf(ColorGreen+"✅ Overall Chapter Completion: %.1f%% (%d of %d chapters)"+ColorReset+"\n", studyProgress, int(completed), int(total))
	fmt.Println("-----------------------------------------------------------------")
}

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
func runDownloader() error {

	if !isCommandAvailable("yt-dlp") || !isCommandAvailable("ffmpeg") {
		return fmt.Errorf("external dependencies missing. Please install 'yt-dlp' and 'ffmpeg' using pacman")
	}
	OUTPUT_DIR :="study_music"

	if err := os.MkdirAll(OUTPUT_DIR, 0755); err != nil {
		return fmt.Errorf("failed to create output directory '%s': %w", OUTPUT_DIR, err)
	}

	fmt.Println(ColorYellow + "\n--- Music Downloader ---" + ColorReset)
	fmt.Printf("Downloads will be saved to the './%s' folder.\n", OUTPUT_DIR)

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("\nEnter YouTube URL: ")

	url, _ := reader.ReadString('\n')
	url = strings.TrimSpace(url)

	if url == "" {
		fmt.Println("No URL entered. Returning to Main Menu...")
		return nil
	}

	fmt.Printf("Processing URL: %s\n", url)
	fmt.Printf("Downloading and converting audio to MP3 (192K quality)...\n")

	cmd := exec.Command("yt-dlp",
		"-x", 
		"--audio-format", "mp3",
		"--audio-quality", "192K",

		"--output", fmt.Sprintf("%s/%%(title)s.%%(ext)s", OUTPUT_DIR),
		url,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("\n❌ Download/Conversion failed for %s. Check yt-dlp output above.\n", url)
		return nil 
	}

	fmt.Printf("\n✅ Success! MP3 file saved in the '%s' directory.\n\n", OUTPUT_DIR)

	return nil
}

func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
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
	configChanged := false

	fmt.Println("\n--- Configure Scheduler Parameters ---")

	newSyllabusEndDate := readDate(reader, "Syllabus Completion Target Date", newConfig.SyllabusEndDate)
	if newSyllabusEndDate != newConfig.SyllabusEndDate { configChanged = true }
	newConfig.SyllabusEndDate = newSyllabusEndDate

	newExamDate := readDate(reader, "Final Exam Date (for reference)", newConfig.ExamDate)
	if newExamDate != newConfig.ExamDate { configChanged = true }
	newConfig.ExamDate = newExamDate

	newDailyStudyHrs := readFloat(reader, "Total Daily Study Hours (Excluding Buffer/Breaks)", newConfig.DailyStudyHrs)
	if newDailyStudyHrs != newConfig.DailyStudyHrs { configChanged = true }
	newConfig.DailyStudyHrs = newDailyStudyHrs

	newMaxSessionHrs := readFloat(reader, "Maximum Hours per Single Session", newConfig.MaxSessionHrs)
	if newMaxSessionHrs != newConfig.MaxSessionHrs { configChanged = true }
	newConfig.MaxSessionHrs = newMaxSessionHrs

	newDailyBufferMins := readInt(reader, "Daily Buffer/Review Time (in minutes)", newConfig.DailyBufferMins)
	if newDailyBufferMins != newConfig.DailyBufferMins { configChanged = true }
	newConfig.DailyBufferMins = newDailyBufferMins

	newWeeklyRestDay := readWeekday(reader, "Weekly Rest Day (e.g., sunday)", newConfig.WeeklyRestDay)
	if newWeeklyRestDay != newConfig.WeeklyRestDay { configChanged = true }
	newConfig.WeeklyRestDay = newWeeklyRestDay

	newRestDayActivity := newConfig.RestDayActivity
	fmt.Printf("Rest Day Activity (Current: %s): ", newConfig.RestDayActivity)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" && input != newConfig.RestDayActivity {
		newRestDayActivity = input
		configChanged = true
	}
	newConfig.RestDayActivity = newRestDayActivity

	if configChanged {
		rawConfig = newConfig
		saveConfig(rawConfig)
		deleteScheduleState() 
		fmt.Printf("\n[INFO] Configuration updated and saved. Max Session Hrs is now: %.1f hrs.\n", rawConfig.MaxSessionHrs)
		fmt.Println("Please run [3] RE-GENERATE to calculate a new study path using these settings.")
	} else {
		fmt.Println("\n[INFO] No configuration changes detected. Schedule state retained.")
	}
	return newConfig
}

func runMainMenu() {
	reader := bufio.NewReader(os.Stdin)
	for {
		rawConfig = loadConfig()
		fmt.Println("\n--- Adaptive NEET Scheduler Menu ---")
		fmt.Println(ColorGreen + "[1] Start TIMER CLI (Daily Study)" + ColorReset)
		fmt.Println(ColorBlue + "[2] View FULL REPORT(Syllabus Status)" + ColorReset)
		fmt.Println(ColorYellow + "[3] RE-GENERATE Schedule (Initialize or Re-balance)" + ColorReset)
		fmt.Println("[4] CHANGE CONFIGURATION (Dates, Times, etc.)")
		fmt.Println("[5] Music Download")
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
			promptConfig(rawConfig)
		case "5":

			if err := runDownloader(); err != nil {
				fmt.Fprintf(os.Stderr, "\n[CRITICAL ERROR] Downloader failed: %v\n", err)
			}
		case "q":
			stopMusic()
			fmt.Println("\nExiting application. Goodbye! 👋")
			return
		default:
			fmt.Println("[ERROR] Invalid choice. Please enter '1', '2', '3', '4', or 'q'.")
		}
	}
}

func main() {

	if len(os.Args) > 1 && os.Args[1] == "generate" {
		rawConfig = loadConfig()
		generateSchedule()
		return
	}

	rawConfig = loadConfig()
	_, initialized := loadState()

	needsGeneration := false

	if !initialized {
		fmt.Println(ColorRed + "[INIT] State file missing or corrupted. Initializing new schedule state." + ColorReset)
		needsGeneration = true
	}

	if needsGeneration {
		fmt.Println("[AUTO-GENERATE] Generating initial schedule...")
		generateSchedule()
	}

	fmt.Println("\n--- Scheduler Ready ---")
	runMainMenu()
}
