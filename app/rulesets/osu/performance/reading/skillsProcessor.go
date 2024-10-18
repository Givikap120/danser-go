package reading

import (
	"github.com/wieku/danser-go/app/beatmap/difficulty"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/preprocessing"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/skills"
)

type SkillsProcessor struct {
	Aim               skills.AimSkill
	AimWithoutSliders skills.AimSkill
	Speed             skills.SpeedSkill
	Flashlight        skills.Flashlight
	ReadingLowAR      skills.ReadingLowAR
	ReadingHighAR     skills.ReadingHighAR
	ReadingHidden     skills.ReadingHidden

	skipIrrelevantToStarRating bool
	isHidden                   bool
}

func NewSkillsProcessor(d *difficulty.Difficulty, stepCalc bool, skipIrrelevantToStarRating bool) *SkillsProcessor {
	obj := &SkillsProcessor{
		Aim:                        *skills.NewAimSkill(d, true, stepCalc),
		AimWithoutSliders:          *skills.NewAimSkill(d, false, stepCalc),
		Speed:                      *skills.NewSpeedSkill(d, stepCalc),
		Flashlight:                 *skills.NewFlashlightSkill(d),
		ReadingLowAR:               *skills.NewReadingLowAR(d, stepCalc),
		ReadingHighAR:              *skills.NewReadingHighAR(d, stepCalc),
		ReadingHidden:              *skills.NewReadingHidden(d, stepCalc),
		skipIrrelevantToStarRating: skipIrrelevantToStarRating,
		isHidden:                   d.CheckModActive(difficulty.Hidden),
	}
	return obj
}

func (skills *SkillsProcessor) Process(current *preprocessing.DifficultyObject) {
	skills.Aim.Process(current)
	skills.Speed.Process(current)
	skills.Flashlight.Process(current)
	skills.ReadingLowAR.Process(current)
	skills.ReadingHighAR.Process(current)

	if skills.skipIrrelevantToStarRating {
		skills.AimWithoutSliders.Process(current)
	}

	if skills.isHidden {
		skills.ReadingHidden.Process(current)
	}
}
