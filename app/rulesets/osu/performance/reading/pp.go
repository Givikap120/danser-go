package reading

import (
	"math"

	"github.com/wieku/danser-go/app/beatmap/difficulty"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/api"
	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/skills"
	"github.com/wieku/danser-go/framework/math/mutils"
)

const (
	PerformanceBaseMultiplier float64 = 1.15
)

/* ------------------------------------------------------------- */
/* pp calc                                                       */

// PPv2 : structure to store ppv2 values
type PPv2 struct {
	attribs api.Attributes

	experimental bool

	scoreMaxCombo      int
	countGreat         int
	countOk            int
	countMeh           int
	countMiss          int
	effectiveMissCount float64

	diff *difficulty.Difficulty

	totalHits                    int
	accuracy                     float64
	amountHitObjectsWithAccuracy int
}

func NewPPCalculator() api.IPerformanceCalculator {
	return &PPv2{}
}

func (pp *PPv2) Calculate(attribs api.Attributes, combo, n300, n100, n50, nmiss int, acc float64, diff *difficulty.Difficulty) api.PPv2Results {
	attribs.MaxCombo = max(1, attribs.MaxCombo)

	if combo < 0 {
		combo = attribs.MaxCombo
	}

	if n300 < 0 {
		n300 = attribs.ObjectCount - n100 - n50 - nmiss
	}

	pp.attribs = attribs
	pp.diff = diff
	pp.totalHits = n300 + n100 + n50 + nmiss
	pp.scoreMaxCombo = combo
	pp.countGreat = n300
	pp.countOk = n100
	pp.countMeh = n50
	pp.countMiss = nmiss
	pp.effectiveMissCount = pp.calculateEffectiveMissCount()
	pp.accuracy = acc

	if diff.CheckModActive(difficulty.ScoreV2 | difficulty.Lazer) {
		pp.amountHitObjectsWithAccuracy = attribs.Circles + attribs.Sliders
	} else {
		pp.amountHitObjectsWithAccuracy = attribs.Circles
	}

	// total pp

	multiplier := PerformanceBaseMultiplier

	if diff.Mods.Active(difficulty.NoFail) {
		multiplier *= max(0.90, 1.0-0.02*pp.effectiveMissCount)
	}

	if diff.Mods.Active(difficulty.SpunOut) && pp.totalHits > 0 {
		multiplier *= 1.0 - math.Pow(float64(attribs.Spinners)/float64(pp.totalHits), 0.85)
	}

	if diff.Mods.Active(difficulty.Relax) {
		okMultiplier := 1.0
		mehMultiplier := 1.0

		if diff.ODReal > 0.0 {
			okMultiplier = max(0.0, 1-math.Pow(diff.ODReal/13.33, 1.8))
			mehMultiplier = max(0.0, 1-math.Pow(diff.ODReal/13.33, 5))
		}

		pp.effectiveMissCount = min(pp.effectiveMissCount+float64(pp.countOk)*okMultiplier+float64(pp.countMeh)*mehMultiplier, float64(pp.totalHits))
	}

	result := pp.calculatePPv2Results()

	pp.effectiveMissCount = 0
	pp.countMiss = 0
	pp.scoreMaxCombo = pp.attribs.MaxCombo
	multiplier *= pp.calculateBalanceAdjustingMultiplier()

	result.Total *= multiplier
	return result
}

func (pp *PPv2) calculatePPv2Results() api.PPv2Results {
	aimValue := pp.computeAimValue()
	speedValue := pp.computeAimValue()

	lowARValue := pp.computeAimValue()
	highARValue := pp.computeAimValue()

	potentialFlashlightValue := pp.computeAimValue()
	hiddenValue := pp.computeAimValue()

	flashlightValue := 0.0
	if pp.diff.Mods.Active(difficulty.Flashlight) {
		flashlightValue = potentialFlashlightValue
	}

	ARValue := math.Pow(
		math.Pow(lowARValue, 1.1)+
			math.Pow(highARValue, 1.1),
		1.0/1.1,
	)

	flashlightARValue := math.Pow(
		math.Pow(ARValue, 1.5)+
			math.Pow(flashlightValue, 1.5),
		1.0/1.5,
	)

	cognitionValue := flashlightARValue + hiddenValue
	mechanicalValue := math.Pow(
		math.Pow(aimValue, 1.1)+
			math.Pow(speedValue, 1.1),
		1.0/1.1,
	)

	cognitionValue = AdjustCognitionPerformance(cognitionValue, mechanicalValue, potentialFlashlightValue)
	accValue := pp.computeAccuracyValue()
	totalValue := cognitionValue + math.Pow(math.Pow(mechanicalValue, 1.1)+math.Pow(accValue, 1.1), 1.0/1.1)

	visualReadingValue := AdjustCognitionPerformance(ARValue+hiddenValue, mechanicalValue, flashlightValue)
	visualFlashlightValue := cognitionValue - visualReadingValue

	results := api.PPv2Results{
		Aim:        aimValue,
		Speed:      speedValue,
		Acc:        accValue,
		Flashlight: visualFlashlightValue,
		Reading:    visualReadingValue,
		Total:      totalValue,
	}

	return results
}

func (pp *PPv2) computeAimValue() float64 {
	aimValue := skills.DefaultDifficultyToPerformance(pp.attribs.Aim)

	// Longer maps are worth more
	lengthBonus := 0.95 + 0.4*min(1.0, float64(pp.totalHits)/2000.0)
	if pp.totalHits > 2000 {
		lengthBonus += math.Log10(float64(pp.totalHits)/2000.0) * 0.5
	}

	aimValue *= lengthBonus

	// Penalize misses by assessing # of misses relative to the total # of objects. Default a 3% reduction for any # of misses.
	if pp.effectiveMissCount > 0 {
		aimValue *= pp.calculateMissPenalty(pp.effectiveMissCount, pp.attribs.AimDifficultStrainCount)
	}

	approachRateFactor := 0.0
	if pp.diff.ARReal > 10.33 {
		approachRateFactor = 0.3 * (pp.diff.ARReal - 10.33)
	} else if pp.diff.ARReal < 8.0 {
		approachRateFactor = 0.05 * (8.0 - pp.diff.ARReal)
	}

	if pp.diff.CheckModActive(difficulty.Relax) {
		approachRateFactor = 0.0
	}

	aimValue *= 1.0 + approachRateFactor*lengthBonus

	// We want to give more reward for lower AR when it comes to aim and HD. This nerfs high AR and buffs lower AR.
	if pp.diff.Mods.Active(difficulty.Hidden) {
		aimValue *= 1.0 + 0.04*(12.0-pp.diff.ARReal)
	}

	// We assume 15% of sliders in a map are difficult since there's no way to tell from the performance calculator.
	estimateDifficultSliders := float64(pp.attribs.Sliders) * 0.15

	if pp.attribs.Sliders > 0 {
		estimateSliderEndsDropped := mutils.Clamp(float64(min(pp.countOk+pp.countMeh+pp.countMiss, pp.attribs.MaxCombo-pp.scoreMaxCombo)), 0, estimateDifficultSliders)
		sliderNerfFactor := (1-pp.attribs.SliderFactor)*math.Pow(1-estimateSliderEndsDropped/estimateDifficultSliders, 3) + pp.attribs.SliderFactor
		aimValue *= sliderNerfFactor
	}

	aimValue *= pp.accuracy
	// It is important to also consider accuracy difficulty when doing that
	aimValue *= 0.98 + math.Pow(pp.diff.ODReal, 2)/2500

	return aimValue
}

func (pp *PPv2) computeSpeedValue() float64 {
	if pp.diff.CheckModActive(difficulty.Relax) {
		return 0
	}

	speedValue := skills.DefaultDifficultyToPerformance(pp.attribs.Speed)

	// Longer maps are worth more
	lengthBonus := 0.95 + 0.4*min(1.0, float64(pp.totalHits)/2000.0)
	if pp.totalHits > 2000 {
		lengthBonus += math.Log10(float64(pp.totalHits)/2000.0) * 0.5
	}

	speedValue *= lengthBonus

	// Penalize misses by assessing # of misses relative to the total # of objects. Default a 3% reduction for any # of misses.
	if pp.effectiveMissCount > 0 {
		speedValue *= pp.calculateMissPenalty(pp.effectiveMissCount, pp.attribs.SpeedDifficultStrainCount)
	}

	approachRateFactor := 0.0
	if pp.diff.ARReal > 10.33 {
		approachRateFactor = 0.3 * (pp.diff.ARReal - 10.33)
	}

	speedValue *= 1.0 + approachRateFactor*lengthBonus

	if pp.diff.Mods.Active(difficulty.Hidden) {
		speedValue *= 1.0 + 0.04*(12.0-pp.diff.ARReal)
	}

	relevantAccuracy := 0.0
	if pp.attribs.SpeedNoteCount != 0 {
		relevantTotalDiff := float64(pp.totalHits) - pp.attribs.SpeedNoteCount
		relevantCountGreat := max(0, float64(pp.countGreat)-relevantTotalDiff)
		relevantCountOk := max(0, float64(pp.countOk)-max(0, relevantTotalDiff-float64(pp.countGreat)))
		relevantCountMeh := max(0, float64(pp.countMeh)-max(0, relevantTotalDiff-float64(pp.countGreat)-float64(pp.countOk)))
		relevantAccuracy = (relevantCountGreat*6.0 + relevantCountOk*2.0 + relevantCountMeh) / (pp.attribs.SpeedNoteCount * 6.0)
	}

	// Scale the speed value with accuracy and OD
	speedValue *= (0.95 + math.Pow(pp.diff.ODReal, 2)/750) * math.Pow((pp.accuracy+relevantAccuracy)/2.0, (14.5-pp.diff.ODReal)/2)

	// Scale the speed value with # of 50s to punish doubletapping.
	if float64(pp.countMeh) >= float64(pp.totalHits)/500 {
		speedValue *= math.Pow(0.99, float64(pp.countMeh)-float64(pp.totalHits)/500.0)
	}

	return speedValue
}

func (pp *PPv2) computeAccuracyValue() float64 {
	if pp.diff.Mods.Active(difficulty.Relax) {
		return 0.0
	}

	// This percentage only considers HitCircles of any value - in this part of the calculation we focus on hitting the timing hit window
	betterAccuracyPercentage := 0.0

	if pp.amountHitObjectsWithAccuracy > 0 {
		betterAccuracyPercentage = float64((pp.countGreat-(pp.totalHits-pp.amountHitObjectsWithAccuracy))*6+pp.countOk*2+pp.countMeh) / (float64(pp.amountHitObjectsWithAccuracy) * 6)
	}

	// It is possible to reach a negative accuracy with this formula. Cap it at zero - zero points
	if betterAccuracyPercentage < 0 {
		betterAccuracyPercentage = 0
	}

	// Lots of arbitrary values from testing.
	// Considering to use derivation from perfect accuracy in a probabilistic manner - assume normal distribution
	accuracyValue := math.Pow(1.52163, pp.diff.ODReal) * math.Pow(betterAccuracyPercentage, 24) * 2.83

	// Bonus for many hitcircles - it's harder to keep good accuracy up for longer
	accuracyValue *= min(1.15, math.Pow(float64(pp.amountHitObjectsWithAccuracy)/1000.0, 0.3))

	if pp.diff.Mods.Active(difficulty.Hidden) {
		accuracyValue *= 1.08
	}

	if pp.diff.Mods.Active(difficulty.Flashlight) {
		accuracyValue *= 1.02
	}

	return accuracyValue
}

func (pp *PPv2) computeFlashlightValue() float64 {
	flashlightValue := skills.FlashlightDifficultyToPerformance(pp.attribs.Flashlight)

	// Penalize misses by assessing # of misses relative to the total # of objects. Default a 3% reduction for any # of misses.
	if pp.effectiveMissCount > 0 {
		flashlightValue *= 0.97 * math.Pow(1-math.Pow(pp.effectiveMissCount/float64(pp.totalHits), 0.775), math.Pow(pp.effectiveMissCount, 0.875))
	}

	// Combo scaling.
	flashlightValue *= pp.getComboScalingFactor()

	// Account for shorter maps having a higher ratio of 0 combo/100 combo flashlight radius.
	scale := 0.7 + 0.1*min(1.0, float64(pp.totalHits)/200.0)
	if pp.totalHits > 200 {
		scale += 0.2 * min(1.0, float64(pp.totalHits-200)/200.0)
	}

	flashlightValue *= scale

	// Scale the flashlight value with accuracy _slightly_.
	flashlightValue *= 0.5 + pp.accuracy/2.0
	// It is important to also consider accuracy difficulty when doing that.
	flashlightValue *= 0.98 + math.Pow(pp.diff.ODReal, 2)/2500

	return flashlightValue
}

func (pp *PPv2) computeLowARValue() float64 {
	readingValue := skills.LowARDifficultyToPerformance(pp.attribs.ReadingDifficultyLowAR)

	// Penalize misses by assessing # of misses relative to the total # of objects. Default a 3% reduction for any # of misses.
	if pp.effectiveMissCount > 0 {
		readingValue *= pp.calculateMissPenalty(pp.effectiveMissCount, pp.attribs.LowArDifficultStrainCount)
	}

	readingValue *= pp.accuracy * pp.accuracy
	readingValue *= math.Pow(0.98+math.Pow(pp.diff.ODReal, 2)/2500, 2)

	return readingValue
}

func (pp *PPv2) computeHighARValue() float64 {
	highARValue := skills.DefaultDifficultyToPerformance(pp.attribs.ReadingDifficultyHighAR)

	// Approximate how much of high AR difficulty is aim
	aimPerformance := skills.DefaultDifficultyToPerformance(pp.attribs.Aim)
	speedPerformance := skills.DefaultDifficultyToPerformance(pp.attribs.Speed)

	aimRatio := aimPerformance / (aimPerformance + speedPerformance)

	// Aim part calculation
	aimPartValue := highARValue * aimRatio
	// We assume 15% of sliders in a map are difficult since there's no way to tell from the performance calculator.
	estimateDifficultSliders := float64(pp.attribs.Sliders) * 0.15

	if pp.attribs.Sliders > 0 {
		estimateSliderEndsDropped := mutils.Clamp(float64(min(pp.countOk+pp.countMeh+pp.countMiss, pp.attribs.MaxCombo-pp.scoreMaxCombo)), 0, estimateDifficultSliders)
		sliderNerfFactor := (1-pp.attribs.SliderFactor)*math.Pow(1-estimateSliderEndsDropped/estimateDifficultSliders, 3) + pp.attribs.SliderFactor
		aimPartValue *= sliderNerfFactor
	}

	aimPartValue *= pp.accuracy
	// It is important to also consider accuracy difficulty when doing that
	aimPartValue *= 0.98 + math.Pow(pp.diff.ODReal, 2)/2500

	// Speed part calculation
	speedPartValue := highARValue * (1 - aimRatio)
	relevantAccuracy := 0.0
	if pp.attribs.SpeedNoteCount != 0 {
		relevantTotalDiff := float64(pp.totalHits) - pp.attribs.SpeedNoteCount
		relevantCountGreat := max(0, float64(pp.countGreat)-relevantTotalDiff)
		relevantCountOk := max(0, float64(pp.countOk)-max(0, relevantTotalDiff-float64(pp.countGreat)))
		relevantCountMeh := max(0, float64(pp.countMeh)-max(0, relevantTotalDiff-float64(pp.countGreat)-float64(pp.countOk)))
		relevantAccuracy = (relevantCountGreat*6.0 + relevantCountOk*2.0 + relevantCountMeh) / (pp.attribs.SpeedNoteCount * 6.0)
	}

	// Scale the speed value with accuracy and OD
	speedPartValue *= (0.95 + math.Pow(pp.diff.ODReal, 2)/750) * math.Pow((pp.accuracy+relevantAccuracy)/2.0, (14.5-pp.diff.ODReal)/2)

	// Scale the speed value with # of 50s to punish doubletapping.
	if float64(pp.countMeh) >= float64(pp.totalHits)/500 {
		speedPartValue *= math.Pow(0.99, float64(pp.countMeh)-float64(pp.totalHits)/500.0)
	}

	return aimPartValue + speedPartValue
}

func (pp *PPv2) computeHiddenValue() float64 {
	if !pp.diff.CheckModActive(difficulty.Hidden) {
		return 0
	}

	readingValue := skills.HiddenDifficultyToPerformance(pp.attribs.HiddenDifficulty)

	lengthBonus := 0.95 + 0.4*min(1.0, float64(pp.totalHits)/2000.0)
	if pp.totalHits > 2000 {
		lengthBonus += math.Log10(float64(pp.totalHits)/2000.0) * 0.5
	}

	readingValue *= lengthBonus

	// Penalize misses by assessing # of misses relative to the total # of objects. Default a 3% reduction for any # of misses.
	if pp.effectiveMissCount > 0 {
		readingValue *= pp.calculateMissPenalty(pp.effectiveMissCount, pp.attribs.HiddenDifficultStrainCount)
	}

	readingValue *= pp.accuracy * pp.accuracy
	readingValue *= 0.98 + math.Pow(pp.diff.ODReal, 2)/2500

	return readingValue
}

func (pp *PPv2) calculateEffectiveMissCount() float64 {
	// guess the number of misses + slider breaks from combo
	comboBasedMissCount := 0.0

	if pp.attribs.Sliders > 0 {
		fullComboThreshold := float64(pp.attribs.MaxCombo) - 0.1*float64(pp.attribs.Sliders)
		if float64(pp.scoreMaxCombo) < fullComboThreshold {
			comboBasedMissCount = fullComboThreshold / max(1.0, float64(pp.scoreMaxCombo))
		}
	}

	// Clamp miss count to maximum amount of possible breaks
	comboBasedMissCount = min(comboBasedMissCount, float64(pp.countOk+pp.countMeh+pp.countMiss))

	return max(float64(pp.countMiss), comboBasedMissCount)
}

func (pp *PPv2) calculateMissPenalty(missCount, difficultStrainCount float64) float64 {
	return 0.96 / ((missCount / (4 * math.Pow(math.Log(difficultStrainCount), 0.94))) + 1)
}

func (pp *PPv2) getComboScalingFactor() float64 {
	if pp.attribs.MaxCombo <= 0 {
		return 1.0
	} else {
		return min(math.Pow(float64(pp.scoreMaxCombo), 0.8)/math.Pow(float64(pp.attribs.MaxCombo), 0.8), 1.0)
	}
}

func AdjustCognitionPerformance(cognitionPerformance, mechanicalPerformance, flashlightPerformance float64) float64 {
	// Assuming that less than 25 pp is not worthy for memory
	capPerformance := mechanicalPerformance + flashlightPerformance + 25

	ratio := cognitionPerformance / capPerformance
	if ratio > 50 {
		return capPerformance
	}

	ratio = softmin(ratio*10, 10, 5) / 10
	return ratio * capPerformance
}

func (pp *PPv2) calculateBalanceAdjustingMultiplier() float64 {
	totalValue := pp.calculatePPv2Results().Total * PerformanceBaseMultiplier

	if totalValue < 600 {
		return 1
	}

	rescaledValue := (totalValue - 600) / 1000
	result := min(0.06*rescaledValue, 0.088*math.Pow(rescaledValue, 0.4))
	return 1 + result
}

// softmin is a function that computes a soft minimum between two values with an optional power argument
func softmin(a, b, power float64) float64 {
	return a * b / math.Log(math.Pow(power, a)+math.Pow(power, b))
}
