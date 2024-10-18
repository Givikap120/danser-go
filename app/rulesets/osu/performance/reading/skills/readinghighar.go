package skills

import (
	"math"

	"github.com/wieku/danser-go/app/beatmap/difficulty"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/evaluators"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/preprocessing"
)

const (
	readingHighARSkillMultiplier    = 9.3
	componentDefaultValueMultiplier = 280
	sumPower                        = 1.1 // Assuming based on OsuDifficultyCalculator.SUM_POWER
	curvePower                      = 3.0369
	curveMultiplier                 = 3.69656
)

// ReadingHighAR represents the skill for reading high AR, inheriting from GraphSkill
type ReadingHighAR struct {
	*Skill
	aimComponent      *HighARAimComponent
	speedComponent    *HighARSpeedComponent
	objectsCount      int
	objectsPreemptSum float64
}

// NewReadingHighAR creates a new instance of ReadingHighAR
func NewReadingHighAR(d *difficulty.Difficulty, stepCalc bool) *ReadingHighAR {
	return &ReadingHighAR{
		Skill:          NewSkill(d, false),
		aimComponent:   NewHighARAimComponent(d, stepCalc),
		speedComponent: NewHighARSpeedComponent(d, stepCalc),
	}
}

// Process handles the current DifficultyObject for strain calculation
func (skill *ReadingHighAR) Process(current *preprocessing.DifficultyObject) {
	skill.aimComponent.Process(current)
	skill.speedComponent.Process(current)

	if !current.IsSpinner {
		skill.objectsCount++
		skill.objectsPreemptSum += current.Preempt
	}

	mergedDifficulty := math.Pow(
		math.Pow(skill.aimComponent.currentSectionPeak, sumPower)+
			math.Pow(skill.speedComponent.currentSectionPeak, sumPower), 1.0/sumPower)

	mergedDifficulty = readingHighARSkillMultiplier * math.Pow(mergedDifficulty, evaluators.MECHANICAL_PP_POWER)

	if current.Index == 0 {
		skill.currentSectionEnd = math.Ceil(current.StartTime/skill.SectionLength) * skill.SectionLength
	}

	for current.StartTime > skill.currentSectionEnd {
		skill.strainPeaks = append(skill.strainPeaks, skill.currentSectionPeak)
		skill.currentSectionPeak = 0
		skill.currentSectionEnd += skill.SectionLength
	}

	skill.currentSectionPeak = math.Max(mergedDifficulty, skill.currentSectionPeak)
}

// DifficultyValue calculates the difficulty value for ReadingHighAR
func (skill *ReadingHighAR) DifficultyValue() float64 {
	difficultyMultiplier := 0.0668

	aimValue := math.Sqrt(skill.aimComponent.DifficultyValue()) * difficultyMultiplier
	speedValue := math.Sqrt(skill.speedComponent.DifficultyValue()) * difficultyMultiplier

	aimPerformance := HighARDifficultyToPerformance(aimValue)
	speedPerformance := HighARDifficultyToPerformance(speedValue)

	totalPerformance := math.Pow(math.Pow(aimPerformance, sumPower)+math.Pow(speedPerformance, sumPower), 1.0/sumPower)

	lengthBonus := 0.95 + 0.4*min(1.0, float64(skill.objectsCount)/2000.0)
	if skill.objectsCount > 2000 {
		lengthBonus += math.Log10(float64(skill.objectsCount)/2000.0) * 0.5
	}
	lengthBonus = math.Pow(lengthBonus, 0.5/evaluators.MECHANICAL_PP_POWER)

	averagePreempt := skill.objectsPreemptSum / float64(skill.objectsCount) / 1000
	lengthBonusPower := 1 + 0.75*math.Pow(0.1, math.Pow(2.3*averagePreempt, 8))

	if lengthBonus < 1 {
		lengthBonusPower = 2
	}

	totalPerformance *= math.Pow(lengthBonus, lengthBonusPower)
	adjustedDifficulty := performanceToDifficulty(totalPerformance)
	difficultyValue := math.Pow(adjustedDifficulty/difficultyMultiplier, 2.0)

	skill.difficulty = readingHighARSkillMultiplier * math.Pow(difficultyValue, evaluators.MECHANICAL_PP_POWER)

	return skill.difficulty
}

// DifficultyToPerformance converts the difficulty value to performance points
func HighARDifficultyToPerformance(difficulty float64) float64 {
	return math.Pow(difficulty, curvePower) * curveMultiplier
}

func performanceToDifficulty(performance float64) float64 {
	return math.Pow(performance/curveMultiplier, 1.0/curvePower)
}

// HighARAimComponent represents the Aim component of ReadingHighAR
type HighARAimComponent struct {
	*AimSkill
}

func NewHighARAimComponent(d *difficulty.Difficulty, stepCalc bool) *HighARAimComponent {
	skill := &HighARAimComponent{AimSkill: NewAimSkill(d, true, stepCalc)}
	skill.StrainValueOf = skill.aimStrainValue
	return skill
}

// Process processes the current DifficultyObject for aim
func (component *HighARAimComponent) aimStrainValue(current *preprocessing.DifficultyObject) float64 {
	component.CurrentStrain *= component.strainDecay(current.DeltaTime)
	aimDifficulty := evaluators.EvaluateAim(current, true) * aimSkillMultiplier
	aimDifficulty *= evaluators.EvaluateHighARDifficultyOf(current, true)

	component.CurrentStrain += aimDifficulty + componentDefaultValueMultiplier*evaluators.EvaluateHighARDifficultyOf(current, true)

	return component.CurrentStrain
}

// HighARSpeedComponent represents the Speed component of ReadingHighAR
type HighARSpeedComponent struct {
	*SpeedSkill
}

func NewHighARSpeedComponent(d *difficulty.Difficulty, stepCalc bool) *HighARSpeedComponent {
	skill := &HighARSpeedComponent{SpeedSkill: NewSpeedSkill(d, stepCalc)}
	skill.StrainValueOf = skill.speedStrainValue
	return skill
}

// Process processes the current DifficultyObject for speed
func (component *HighARSpeedComponent) speedStrainValue(current *preprocessing.DifficultyObject) float64 {
	component.CurrentStrain *= component.strainDecay(current.DeltaTime)
	speedDifficulty := evaluators.EvaluateSpeed(current) * speedSkillMultiplier
	speedDifficulty *= evaluators.EvaluateHighARDifficultyOf(current, false)

	component.CurrentStrain += speedDifficulty + componentDefaultValueMultiplier*evaluators.EvaluateHighARDifficultyOf(current, false)
	component.CurrentRhythm = current.RhythmDifficulty

	return component.CurrentStrain
}
