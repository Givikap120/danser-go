package reading

import (
	"log"
	"math"
	"time"

	"github.com/wieku/danser-go/app/beatmap/difficulty"
	"github.com/wieku/danser-go/app/beatmap/objects"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/api"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/preprocessing"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/skills"
)

const (
	// StarScalingFactor is a global stars multiplier
	StarScalingFactor float64 = 0.0668
	CurrentVersion    int     = 20241018
)

type DifficultyCalculator struct{}

func NewDifficultyCalculator() api.IDifficultyCalculator {
	return &DifficultyCalculator{}
}

// getStarsFromRawValues converts raw skill values to Attributes
func (diffCalc *DifficultyCalculator) getStarsFromRawValues(rawAim, rawAimNoSliders, rawSpeed, rawFlashlight, rawLowAR, rawHighAR, rawHidden float64, diff *difficulty.Difficulty, attr api.Attributes) api.Attributes {
	aimRating := math.Sqrt(rawAim) * StarScalingFactor
	aimRatingNoSliders := math.Sqrt(rawAimNoSliders) * StarScalingFactor
	speedRating := math.Sqrt(rawSpeed) * StarScalingFactor
	flashlightRating := math.Sqrt(rawFlashlight) * StarScalingFactor

	lowARRating := math.Sqrt(rawLowAR) * StarScalingFactor
	highARRating := math.Sqrt(rawHighAR) * StarScalingFactor
	hiddenRating := math.Sqrt(rawHidden) * StarScalingFactor

	sliderFactor := 1.0
	if aimRating > 0.00001 {
		sliderFactor = aimRatingNoSliders / aimRating
	}

	if diff.CheckModActive(difficulty.TouchDevice) {
		aimRating = math.Pow(aimRating, 0.8)
		flashlightRating = math.Pow(flashlightRating, 0.8)

		lowARRating = math.Pow(lowARRating, 0.8)
		highARRating = math.Pow(highARRating, 0.9)
		hiddenRating = math.Pow(hiddenRating, 0.8)
	}

	if diff.CheckModActive(difficulty.Relax) {
		aimRating *= 0.9
		speedRating = 0
		flashlightRating *= 0.7

		lowARRating *= 0.95
		highARRating *= 0.7
		hiddenRating *= 0.7
	}

	var total float64

	baseAimPerformance := skills.DefaultDifficultyToPerformance(aimRating)
	baseSpeedPerformance := skills.DefaultDifficultyToPerformance(speedRating)

	baseLowARPerformance := skills.LowARDifficultyToPerformance(lowARRating)
	baseHighARPerformance := skills.HighARDifficultyToPerformance(highARRating)

	potentialFlashlightPerformance := skills.FlashlightDifficultyToPerformance(flashlightRating)

	baseFlashlightPerformance := 0.0
	baseHiddenPerformance := 0.0

	if diff.CheckModActive(difficulty.Flashlight) {
		baseFlashlightPerformance = potentialFlashlightPerformance
	}

	if diff.CheckModActive(difficulty.Hidden) {
		baseHiddenPerformance = skills.HiddenDifficultyToPerformance(hiddenRating)
	}

	baseARPerformance := math.Pow(
		math.Pow(baseLowARPerformance, 1.1)+
			math.Pow(baseHighARPerformance, 1.1),
		1.0/1.1,
	)

	baseFlashlightARPerformance := math.Pow(
		math.Pow(baseARPerformance, 1.5)+
			math.Pow(baseFlashlightPerformance, 1.5),
		1.0/1.5,
	)

	baseCognitionPerformance := baseFlashlightARPerformance + baseHiddenPerformance
	baseMechanicalPerformance := math.Pow(
		math.Pow(baseAimPerformance, 1.1)+
			math.Pow(baseSpeedPerformance, 1.1),
		1.0/1.1,
	)

	baseCognitionPerformance = AdjustCognitionPerformance(baseCognitionPerformance, baseMechanicalPerformance, potentialFlashlightPerformance)
	basePerformance := baseMechanicalPerformance + baseCognitionPerformance

	if basePerformance > 0.00001 {
		total = math.Cbrt(PerformanceBaseMultiplier) * 0.027 * (math.Cbrt(100000/math.Pow(2, 1/1.1)*basePerformance) + 4)
	}

	attr.Total = total
	attr.Aim = aimRating
	attr.SliderFactor = sliderFactor
	attr.Speed = speedRating
	attr.Flashlight = flashlightRating

	attr.ReadingDifficultyLowAR = lowARRating
	attr.ReadingDifficultyHighAR = hiddenRating
	attr.HiddenDifficulty = hiddenRating

	return attr
}

// Retrieves skill values and converts to Attributes
func (diffCalc *DifficultyCalculator) getStars(skills *SkillsProcessor, diff *difficulty.Difficulty, attr api.Attributes) api.Attributes {
	attr = diffCalc.getStarsFromRawValues(
		skills.Aim.DifficultyValue(),
		skills.AimWithoutSliders.DifficultyValue(),
		skills.Speed.DifficultyValue(),
		skills.Flashlight.DifficultyValue(),
		skills.ReadingLowAR.DifficultyValue(),
		skills.ReadingHighAR.DifficultyValue(),
		skills.ReadingHidden.DifficultyValue(),
		diff,
		attr,
	)

	attr.SpeedNoteCount = skills.Speed.RelevantNoteCount()
	attr.AimDifficultStrainCount = skills.Aim.CountDifficultStrains()
	attr.SpeedDifficultStrainCount = skills.Speed.CountDifficultStrains()

	attr.LowArDifficultStrainCount = skills.ReadingLowAR.CountDifficultStrains()
	attr.HiddenDifficultStrainCount = skills.ReadingHidden.CountDifficultStrains()

	return attr
}

func (diffCalc *DifficultyCalculator) addObjectToAttribs(o objects.IHitObject, attr *api.Attributes) {
	if s, ok := o.(*objects.Slider); ok {
		attr.Sliders++
		attr.MaxCombo += len(s.ScorePoints)
	} else if _, ok := o.(*objects.Circle); ok {
		attr.Circles++
	} else if _, ok := o.(*objects.Spinner); ok {
		attr.Spinners++
	}

	attr.MaxCombo++
	attr.ObjectCount++
}

// CalculateSingle calculates the final difficultyapi.Attributes of a map
func (diffCalc *DifficultyCalculator) CalculateSingle(objects []objects.IHitObject, diff *difficulty.Difficulty) api.Attributes {
	diffObjects := preprocessing.CreateDifficultyObjects(objects, diff)

	skills := NewSkillsProcessor(diff, false, false)

	attr := api.Attributes{}

	diffCalc.addObjectToAttribs(objects[0], &attr)

	for i, o := range diffObjects {
		diffCalc.addObjectToAttribs(objects[i+1], &attr)

		skills.Process(o)
	}

	return diffCalc.getStars(skills, diff, attr)
}

// CalculateStep calculates successive star ratings for every part of a beatmap
func (diffCalc *DifficultyCalculator) CalculateStep(objects []objects.IHitObject, diff *difficulty.Difficulty) []api.Attributes {
	modString := difficulty.GetDiffMaskedMods(diff.Mods).String()
	if modString == "" {
		modString = "NM"
	}

	log.Println("Calculating step SR for mods:", modString)

	startTime := time.Now()

	diffObjects := preprocessing.CreateDifficultyObjects(objects, diff)

	skills := NewSkillsProcessor(diff, true, false)

	stars := make([]api.Attributes, 1, len(objects))

	diffCalc.addObjectToAttribs(objects[0], &stars[0])

	for i, o := range diffObjects {
		attr := stars[i]
		diffCalc.addObjectToAttribs(objects[i+1], &attr)

		skills.Process(o)

		stars = append(stars, diffCalc.getStars(skills, diff, attr))
	}

	endTime := time.Now()

	log.Println("Calculations finished! Took ", endTime.Sub(startTime).Truncate(time.Millisecond).String())

	return stars
}

func (diffCalc *DifficultyCalculator) CalculateStrainPeaks(objects []objects.IHitObject, diff *difficulty.Difficulty) api.StrainPeaks {
	diffObjects := preprocessing.CreateDifficultyObjects(objects, diff)

	skills := NewSkillsProcessor(diff, false, true)

	for _, o := range diffObjects {
		skills.Process(o)
	}

	peaks := api.StrainPeaks{
		Aim:           skills.Aim.GetCurrentStrainPeaks(),
		Speed:         skills.Speed.GetCurrentStrainPeaks(),
		Flashlight:    skills.Flashlight.GetCurrentStrainPeaks(),
		ReadingLowAR:  skills.ReadingLowAR.GetCurrentStrainPeaks(),
		ReadingHighAR: skills.ReadingHighAR.GetCurrentStrainPeaks(),
		ReadingHidden: skills.ReadingHidden.GetCurrentStrainPeaks(),
	}

	peaks.Total = make([]float64, len(peaks.Aim))

	for i := 0; i < len(peaks.Aim); i++ {
		stars := diffCalc.getStarsFromRawValues(peaks.Aim[i], peaks.Aim[i], peaks.Speed[i], peaks.Flashlight[i], peaks.ReadingLowAR[i], peaks.ReadingHighAR[i], peaks.ReadingHidden[i], diff, api.Attributes{})
		peaks.Total[i] = stars.Total
	}

	return peaks
}

func (diffCalc *DifficultyCalculator) GetVersion() int {
	return CurrentVersion
}

func (diffCalc *DifficultyCalculator) GetVersionMessage() string {
	return "2024-10-18: reading rework"
}
