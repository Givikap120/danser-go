package skills

import (
	"math"

	"github.com/wieku/danser-go/app/beatmap/difficulty"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/evaluators"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/preprocessing"
	"github.com/wieku/danser-go/framework/math/mutils"
)

// Constants for ReadingLowAR
const (
	readingLowARSkillMultiplier        float64 = 1.23
	readingLowARAimComponentMultiplier         = 0.4
	strainDecayBase                    float64 = 0.15
)

// ReadingLowAR represents the skill for low AR reading, inheriting from Skill
type ReadingLowAR struct {
	*Skill
	currentDensityAimStrain float64
}

// NewReadingLowAR creates a new instance of ReadingLowAR
func NewReadingLowAR(d *difficulty.Difficulty) *ReadingLowAR {
	skill := &ReadingLowAR{Skill: NewSkill(d, false)}
	skill.ReducedSectionCount = 5
	skill.ReducedStrainBaseline = 0.7
	return skill
}

// strainDecay for ReadingLowAR
func (skill *ReadingLowAR) strainDecay(ms float64) float64 {
	return math.Pow(strainDecayBase, ms/1000)
}

// Process handles the current DifficultyObject for strain calculation
func (skill *ReadingLowAR) Process(current *preprocessing.DifficultyObject) {
	densityReadingDifficulty := evaluators.EvaluateReadingLowARDifficultyOf(current)
	densityAimingFactor := evaluators.EvaluateAimingDensityFactorOf(current)

	skill.currentDensityAimStrain *= skill.strainDecay(current.DeltaTime)
	skill.currentDensityAimStrain += densityAimingFactor * evaluators.EvaluateAim(current, true) * readingLowARAimComponentMultiplier

	totalDensityDifficulty := (skill.currentDensityAimStrain + densityReadingDifficulty) * readingLowARSkillMultiplier

	skill.objectStrains = append(skill.objectStrains, totalDensityDifficulty)
	skill.strainPeaksSorted.Add(totalDensityDifficulty)

	if current.Index == 0 {
		skill.currentSectionEnd = math.Ceil(current.StartTime/skill.SectionLength) * skill.SectionLength
	}

	for current.StartTime > skill.currentSectionEnd {
		skill.strainPeaks = append(skill.strainPeaks, skill.currentSectionPeak)
		skill.currentSectionPeak = 0
		skill.currentSectionEnd += skill.SectionLength
	}

	skill.currentSectionPeak = math.Max(totalDensityDifficulty, skill.currentSectionPeak)

	if !skill.stepCalc {
		return
	}

	skill.difficultyValue()

	if skill.lastDifficulty != skill.difficulty {
		skill.difficultStrainCount = skill.countDifficultStrains()
	} else if skill.difficulty != 0 {
		skill.difficultStrainCount += 1.1 / (1 + math.Exp(-10*(currentStrain/(skill.difficulty/10)-0.88)))
	}

	skill.lastDifficulty = skill.difficulty
}

// DifficultyValue calculates the difficulty value for ReadingLowAR
func (skill *ReadingLowAR) difficultyValue() float64 {
	if skill.peakWeights == nil { //Precalculated peak weights
		skill.peakWeights = make([]float64, skill.ReducedSectionCount)
		for i := range skill.ReducedSectionCount {
			scale := math.Log10(mutils.Lerp(1.0, 10.0, mutils.Clamp(float64(i)/float64(skill.ReducedSectionCount), 0, 1)))
			skill.peakWeights[i] = mutils.Lerp(skill.ReducedStrainBaseline, 1.0, scale)
		}
	}

	skill.difficulty = 0.0
	weight := 1.0

	strains := skill.getCurrentStrainPeaksSorted()

	lowest := strains[len(strains)-1]

	for i := range min(len(strains), skill.ReducedSectionCount) {
		strains[len(strains)-1-i] *= skill.peakWeights[i]
		lowest = min(lowest, strains[len(strains)-1-i])
	}

	// Search for lowest strain that's higher or equal than lowest reduced strain to avoid unnecessary sorting
	idx, _ := slices.BinarySearch(strains, lowest)
	slices.Sort(strains[idx:])

	lastDiff := -math.MaxFloat64

	for i := range len(strains) {
		skill.difficulty += strains[len(strains)-1-i] * weight
		weight *= skill.DecayWeight

		if math.Abs(skill.difficulty-lastDiff) < math.SmallestNonzeroFloat64 { // escape when strain * weight calculates to 0
			break
		}

		lastDiff = skill.difficulty
	}

	return skill.difficulty
}

// DifficultyToPerformance converts the difficulty value to performance points
func LowARDifficultyToPerformance(difficulty float64) float64 {
	return math.Max(
		math.Max(math.Pow(difficulty, 1.5)*20, math.Pow(difficulty, 2)*17.0),
		math.Max(math.Pow(difficulty, 3)*10.5, math.Pow(difficulty, 4)*6.00),
	)
}
