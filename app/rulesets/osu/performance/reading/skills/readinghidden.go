package skills

import (
	"math"

	"github.com/wieku/danser-go/app/beatmap/difficulty"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/evaluators"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/preprocessing"
)

const (
	readingHiddenSkillMultiplier float64 = 7.632
)

// ReadingHidden represents the ReadingHidden skill, inheriting from AimSkill
type ReadingHidden struct {
	*AimSkill
}

// NewReadingHidden creates a new instance of ReadingHidden
func NewReadingHidden(d *difficulty.Difficulty, stepCalc bool) *ReadingHidden {
	skill := &ReadingHidden{AimSkill: NewAimSkill(d, false, stepCalc)}

	skill.StrainValueOf = skill.readingHiddenStrainValue
	return skill
}

// readingHiddenStrainValue overrides the strain calculation for ReadingHidden
func (skill *ReadingHidden) readingHiddenStrainValue(current *preprocessing.DifficultyObject) float64 {
	skill.currentStrain *= skill.strainDecay(current.DeltaTime)

	// Calculate hidden difficulty without slider aim
	hiddenDifficulty := evaluators.EvaluateAim(current, false)
	hiddenDifficulty *= evaluators.EvaluateHiddenDifficultyOf(current)
	hiddenDifficulty *= readingHiddenSkillMultiplier

	// Add to the current strain
	skill.currentStrain += hiddenDifficulty
	skill.objectStrains = append(skill.objectStrains, skill.currentStrain)

	return skill.currentStrain
}

// DifficultyToPerformance converts difficulty to performance points, similar to C#'s static method
func HiddenDifficultyToPerformance(difficulty float64) float64 {
	return math.Max(
		math.Max(difficulty*16, math.Pow(difficulty, 2)*10),
		math.Pow(difficulty, 3)*4,
	)
}
