package skills

import (
	"math"

	"github.com/wieku/danser-go/app/beatmap/difficulty"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/evaluators"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/preprocessing"
)

const (
	speedSkillMultiplier float64 = 1.430
	speedStrainDecayBase float64 = 0.3
)

type SpeedSkill struct {
	*Skill

	CurrentStrain float64
	CurrentRhythm float64
	maxStrain     float64

	relevantNoteCountV float64
}

func NewSpeedSkill(d *difficulty.Difficulty, stepCalc bool) *SpeedSkill {
	skill := &SpeedSkill{
		Skill: NewSkill(d, stepCalc),
	}

	skill.ReducedSectionCount = 5
	skill.StrainValueOf = skill.speedStrainValue
	skill.CalculateInitialStrain = skill.speedInitialStrain

	return skill
}

func (s *SpeedSkill) strainDecay(ms float64) float64 {
	return math.Pow(speedStrainDecayBase, ms/1000)
}

func (s *SpeedSkill) speedInitialStrain(time float64, current *preprocessing.DifficultyObject) float64 {
	return (s.CurrentStrain * s.CurrentRhythm) * s.strainDecay(time-current.Previous(0).StartTime)
}

func (s *SpeedSkill) speedStrainValue(current *preprocessing.DifficultyObject) float64 {
	s.CurrentStrain *= s.strainDecay(current.StrainTime)
	s.CurrentStrain += evaluators.EvaluateSpeed(current) * speedSkillMultiplier

	if math.IsNaN(current.RhythmDifficulty) {
		current.RhythmDifficulty = evaluators.EvaluateRhythm(current)
	}
	s.CurrentRhythm = current.RhythmDifficulty

	totalStrain := s.CurrentStrain * s.CurrentStrain

	s.objectStrains = append(s.objectStrains, totalStrain)

	if !s.stepCalc { // Don't need to precalculate relevant note count for normal strain calc
		return totalStrain
	}

	if totalStrain > s.maxStrain {
		s.maxStrain = max(s.maxStrain, totalStrain)
		s.relevantNoteCountV = s.relevantNoteCount()
	} else if s.maxStrain != 0 {
		s.relevantNoteCountV += 1.0 / (1.0 + math.Exp(-(totalStrain/s.maxStrain*12.0 - 6.0)))
	}

	return totalStrain
}

func (s *SpeedSkill) relevantNoteCount() (sum float64) {
	if len(s.objectStrains) == 0 {
		return
	}

	if s.maxStrain == 0 {
		return
	}

	for _, strain := range s.objectStrains {
		sum += 1.0 / (1.0 + math.Exp(-(strain/s.maxStrain*12.0 - 6.0)))
	}

	return
}

func (s *SpeedSkill) RelevantNoteCount() float64 {
	if s.stepCalc {
		return s.relevantNoteCountV
	}

	return s.relevantNoteCount()
}
