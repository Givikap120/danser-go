package api

type Attributes struct {
	// Total Star rating, visible on osu!'s beatmap page
	Total float64

	// Aim stars, needed for Performance Points (aka PP) calculations
	Aim float64

	// Speed stars, needed for Performance Points (aka PP) calculations
	Speed float64

	SpeedNoteCount float64

	AimDifficultStrainCount   float64
	SpeedDifficultStrainCount float64

	// Flashlight stars, needed for Performance Points (aka PP) calculations
	Flashlight float64

	// SliderFactor is a ratio of Aim calculated without sliders to Aim with them
	SliderFactor float64

	ReadingDifficultyLowAR  float64
	ReadingDifficultyHighAR float64
	HiddenDifficulty        float64

	LowArDifficultStrainCount  float64
	HiddenDifficultStrainCount float64

	ObjectCount int
	Circles     int
	Sliders     int
	Spinners    int
	MaxCombo    int
}

// StrainPeaks contains peaks of Aim, Speed and Flashlight skills, as well as peaks passed through star rating formula
type StrainPeaks struct {
	// Aim peaks
	Aim []float64

	// Speed peaks
	Speed []float64

	// Flashlight peaks
	Flashlight []float64

	ReadingLowAR  []float64
	ReadingHighAR []float64
	ReadingHidden []float64

	// Total contains aim, speed and flashlight peaks passed through star rating formula
	Total []float64
}

type PPv2Results struct {
	Aim, Speed, Acc, Flashlight, Reading, Total float64
}
