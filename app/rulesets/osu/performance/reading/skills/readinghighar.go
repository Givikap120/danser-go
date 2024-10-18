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
func NewReadingHighAR(d *difficulty.Difficulty) *ReadingHighAR {
	return &ReadingHighAR{
		Skill:          NewSkill(d, false),
		aimComponent:   NewHighARAimComponent(d),
		speedComponent: NewHighARSpeedComponent(d),
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
		math.Pow(skill.aimComponent.CurrentSectionPeak, sumPower)+
			math.Pow(skill.speedComponent.CurrentSectionPeak, sumPower), 1.0/sumPower)

	mergedDifficulty = readingHighARSkillMultiplier * math.Pow(mergedDifficulty, evaluators.MECHANICAL_PP_POWER)

	if current.Index == 0 {
		skill.CurrentSectionEnd = math.Ceil(current.StartTime/skill.SectionLength) * skill.SectionLength
	}

	for current.StartTime > skill.CurrentSectionEnd {
		skill.strainPeaks = append(skill.strainPeaks, skill.CurrentSectionPeak)
		skill.CurrentSectionPeak = 0
		skill.CurrentSectionEnd += skill.SectionLength
	}

	skill.CurrentSectionPeak = math.Max(mergedDifficulty, skill.CurrentSectionPeak)
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

	skill.Difficulty = readingHighARSkillMultiplier * math.Pow(difficultyValue, evaluators.MECHANICAL_PP_POWER)

	return skill.Difficulty
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

func NewHighARAimComponent(d *difficulty.Difficulty) *HighARAimComponent {
	skill := &HighARAimComponent{Aim: NewAimSkill(d, true, false)}
	skill.StrainValueOf = skill.aimStrainValue
	return skill
}

// Process processes the current DifficultyObject for aim
func (component *HighARAimComponent) aimStrainValue(current *preprocessing.DifficultyObject) float64 {
	component.CurrentStrain *= component.strainDecay(current.DeltaTime)
	aimDifficulty := evaluators.EvaluateAim(current, true) * component.aimSkillMultiplier
	aimDifficulty *= evaluators.EvaluateHighARDifficultyOf(current, true)

	component.CurrentStrain += aimDifficulty + componentDefaultValueMultiplier*evaluators.EvaluateHighARDifficultyOf(current, true)

	return component.CurrentStrain
}

// HighARSpeedComponent represents the Speed component of ReadingHighAR
type HighARSpeedComponent struct {
	*SpeedSkill
}

func NewHighARSpeedComponent(d *difficulty.Difficulty) *HighARSpeedComponent {
	skill := &HighARSpeedComponent{Speed: NewSpeedSkill(d, false)}
	skill.StrainValueOf = skill.speedStrainValue
	return skill
}

// Process processes the current DifficultyObject for speed
func (component *HighARSpeedComponent) speedStrainValue(current *preprocessing.DifficultyObject) float64 {
	component.CurrentStrain *= component.strainDecay(current.DeltaTime)
	speedDifficulty := evaluators.EvaluateSpeed(current) * component.speedSkillMultiplier
	speedDifficulty *= evaluators.EvaluateHighARDifficultyOf(current, false)

	component.CurrentStrain += speedDifficulty + componentDefaultValueMultiplier*evaluators.EvaluateHighARDifficultyOf(current, false)
	component.CurrentRhythm = current.RhythmDifficulty

	return component.CurrentStrain
}
