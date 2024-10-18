package evaluators

import (
	"math"
	"sort"

	"github.com/wieku/danser-go/app/rulesets/osu/performance/reading/preprocessing"
	"github.com/wieku/danser-go/framework/math/mutils"
)

const (
	reading_window_size float64 = 3000
	overlap_multiplier  float64 = 1
)

func EvaluateReadingLowARDifficultyOf(current *preprocessing.DifficultyObject) float64 {
	// If the current object is a Spinner or it's the first object (index 0), return 0 difficulty
	if current.IsSpinner || current.Index == 0 {
		return 0
	}

	// Calculate base difficulty
	density := math.Max(1, EvaluateDensityOf(current, true, true, 1.0))
	difficulty := math.Pow(4*math.Log(density), 2.5)

	// Calculate overlap bonus and add it to the difficulty
	overlapBonus := EvaluateOverlapDifficultyOf(current) * difficulty
	difficulty += overlapBonus

	return difficulty
}

func EvaluateHiddenDifficultyOf(currObj *preprocessing.DifficultyObject) float64 {
	density := EvaluateDensityOf(currObj, false, false, 1.0)
	preempt := currObj.Preempt / 1000

	densityFactor := math.Pow(density/6.2, 1.5)

	var invisibilityFactor float64

	// AR11+DT and faster = 0 HD pp unless density is big
	if preempt < 0.2 {
		invisibilityFactor = 0
	} else {
		// Else accelerating growth until around ART0, then linear,
		// and starting from AR5 is 3 times faster again to buff AR0 +HD
		invisibilityFactor = math.Min(math.Pow(preempt*2.4-0.2, 5), math.Max(preempt, preempt*3-2.4))
	}

	hdDifficulty := invisibilityFactor + densityFactor

	// Scale by unpredictability slightly
	hdDifficulty *= 0.96 + 0.1*EvaluateInpredictabilityOf(currObj) // Max multiplier is 1.1

	return hdDifficulty
}

func EvaluateHighARDifficultyOf(currObj *preprocessing.DifficultyObject, applyAdjust bool) float64 {
	result := GetHighARScaling(currObj.Preempt)

	if applyAdjust {
		inpredictability := EvaluateInpredictabilityOf(currObj)

		// Apply nerf if object isn't new combo
		inpredictability *= 1 + 0.1*(800-currObj.FollowLineTime)/800

		result *= 0.98 + 0.6*inpredictability
	}

	return result
}

func GetHighARScaling(preempt float64) float64 {
	// Get preempt in seconds
	preempt /= 1000
	var value float64

	if preempt < 0.375 {
		// We have a stop in the point of AR10.5, the value here = 0.396875, derivative = -10.5833,
		value = 0.63 * math.Pow(8-20*preempt, 2.0/3) // This function is matching live high AR bonus
	} else {
		value = math.Exp(9.07583 - 80.0*preempt/3)
	}

	return math.Pow(value, 1.0/ReadingHighAR.MECHANICAL_PP_POWER)
}

func EvaluateDensityOf(currObj *preprocessing.DifficultyObject, applyDistanceNerf bool, applySliderbodyDensity bool, angleNerfMultiplier float64) float64 {
	density := 0.0
	densityAnglesNerf := -2.0

	prevObj0 := currObj

	readingObjects := currObj.ReadingObjects
	for i := 0; i < len(readingObjects); i++ {
		loopObj := readingObjects[i].HitObject

		if loopObj.Index < 1 {
			continue // Don't look at the first object of the map
		}

		loopDifficulty := currObj.OpacityAt(loopObj.BaseObject.GetStartTime())

		if applyDistanceNerf {
			loopDifficulty *= (logistic((loopObj.MinimumJumpDistance-80)/10) + 0.2) / 1.2
		}

		if applySliderbodyDensity {
			if slider, ok := currObj.BaseObject.(*preprocessing.LazySlider); ok {
				sliderBodyLength := math.Max(1, float64(slider.GetLength())/currObj.Diff.CircleRadiusU)
				sliderBodyLength = math.Min(sliderBodyLength, 1+float64(slider.LazyTravelDistance/8))
				sliderBodyBuff := math.Log10(sliderBodyLength)
				maxBuff := 0.5
				if i > 0 {
					maxBuff += 1
				}
				if i < len(readingObjects)-1 {
					maxBuff += 1
				}
				loopDifficulty *= 1 + 1.5*math.Min(sliderBodyBuff, maxBuff)
			}
		}

		timeBetweenCurrAndLoopObj := currObj.StartTime - loopObj.BaseObject.GetStartTime()
		loopDifficulty *= getTimeNerfFactor(timeBetweenCurrAndLoopObj)

		if loopObj.StrainTime > prevObj0.StrainTime {
			rhythmSimilarity := 1 - getRhythmDifference(loopObj.StrainTime, prevObj0.StrainTime)
			rhythmSimilarity = mutils.Clamp(rhythmSimilarity, 0.5, 0.75)
			rhythmSimilarity = 4 * (rhythmSimilarity - 0.5)
			loopDifficulty *= rhythmSimilarity
		}

		density += loopDifficulty

		angleNerf := (loopObj.AnglePredictability / 2) + 0.5
		densityAnglesNerf += angleNerf * loopDifficulty * angleNerfMultiplier

		prevObj0 = loopObj
	}

	density -= math.Max(0, densityAnglesNerf)
	return density
}

func EvaluateOverlapDifficultyOf(currObj *preprocessing.DifficultyObject) float64 {
	screenOverlapDifficulty := 0.0

	if len(currObj.ReadingObjects) == 0 {
		return 0
	}

	var overlapDifficulties []preprocessing.ReadingObject

	readingObjects := currObj.ReadingObjects

	// Find initial overlap values
	for _, readingObject := range readingObjects {
		loopObj := readingObject.HitObject
		loopReadingObjects := loopObj.ReadingObjects

		if len(loopReadingObjects) == 0 {
			continue
		}

		targetStartTime := currObj.StartTime - currObj.Preempt
		overlapness := boundBinarySearch(loopReadingObjects, targetStartTime)

		if overlapness > 0 {
			overlapDifficulties = append(overlapDifficulties, preprocessing.ReadingObject{HitObject: loopObj, Overlapness: overlapness})
		}
	}

	if len(overlapDifficulties) == 0 {
		return 0
	}

	// Sort difficulties in descending order
	sort.Slice(overlapDifficulties, func(i, j int) bool {
		return overlapDifficulties[i].Overlapness > overlapDifficulties[j].Overlapness
	})

	// Nerf overlap values of easier notes that are in the same place as hard notes
	for i := 0; i < len(overlapDifficulties); i++ {
		harderObject := overlapDifficulties[i]

		// Look for all easier objects
		for j := i + 1; j < len(overlapDifficulties); j++ {
			easierObject := overlapDifficulties[j]

			// Get the overlap value
			overlapValue := 0.0

			// Check overlap values
			if harderObject.HitObject.Index > easierObject.HitObject.Index {
				if val, ok := harderObject.HitObject.OverlapValues[easierObject.HitObject.Index]; ok {
					overlapValue = val
				}
			} else {
				if val, ok := easierObject.HitObject.OverlapValues[harderObject.HitObject.Index]; ok {
					overlapValue = val
				}
			}

			// Nerf easier object if it overlaps in the same place as the harder one
			easierObject.Overlapness *= math.Pow(1-overlapValue, 2)
		}
	}

	const decayWeight = 0.5
	const threshold = 0.6
	weight := 1.0

	// Sum the overlap values to get difficulty
	for _, diffObject := range overlapDifficulties {
		if diffObject.Overlapness > threshold {
			// Add weighted difficulty
			screenOverlapDifficulty += math.Max(0, diffObject.Overlapness-threshold) * weight
			weight *= decayWeight
		}
	}

	return overlap_multiplier * math.Max(0, screenOverlapDifficulty)
}

func EvaluateAimingDensityFactorOf(current *preprocessing.DifficultyObject) float64 {
	// Evaluate density with custom parameters (true, false, 0.5)
	difficulty := EvaluateDensityOf(current, true, false, 0.5)

	// Return the aiming density factor
	return math.Max(0, math.Pow(difficulty, 1.37)-1)
}

func EvaluateInpredictabilityOf(osuCurrObj *preprocessing.DifficultyObject) float64 {
	// Constants for different factors (velocity, angle, rhythm)
	const velocityChangePart = 0.8
	const angleChangePart = 0.1
	const rhythmChangePart = 0.1

	// Check if current or previous object is a Spinner or first object in the map
	if osuCurrObj.IsSpinner || osuCurrObj.Index == 0 {
		return 0
	}

	osuLastObj := osuCurrObj.Previous(0)
	if osuLastObj.IsSpinner {
		return 0
	}

	// Calculate rhythm similarity and apply clamp
	rhythmSimilarity := 1 - getRhythmDifference(osuCurrObj.StrainTime, osuLastObj.StrainTime)
	rhythmSimilarity = math.Min(math.Max(rhythmSimilarity, 0.5), 0.75)
	rhythmSimilarity = 4 * (rhythmSimilarity - 0.5)

	// Calculate velocity change bonus
	velocityChangeBonus := getVelocityChangeFactor(osuCurrObj, osuLastObj) * rhythmSimilarity

	// Calculate velocities for current and last object
	currVelocity := osuCurrObj.LazyJumpDistance / osuCurrObj.StrainTime
	prevVelocity := osuLastObj.LazyJumpDistance / osuLastObj.StrainTime

	// Calculate angle change bonus
	angleChangeBonus := 0.0
	if !math.IsNaN(osuCurrObj.Angle) && !math.IsNaN(osuLastObj.Angle) && currVelocity > 0 && prevVelocity > 0 {
		angleChangeBonus = 1 - osuCurrObj.AnglePredictability
		angleChangeBonus *= math.Min(currVelocity, prevVelocity) / math.Max(currVelocity, prevVelocity) // Prevent cheesing
	}
	angleChangeBonus *= rhythmSimilarity

	// Calculate rhythm change bonus
	rhythmChangeBonus := 0.0
	if osuCurrObj.Index > 1 {
		osuLastLastObj := osuCurrObj.Previous(1)

		currDelta := osuCurrObj.StrainTime
		lastDelta := osuLastObj.StrainTime

		if sliderCurr, ok := osuLastObj.BaseObject.(*preprocessing.LazySlider); ok {
			currDelta -= sliderCurr.GetDuration() / osuCurrObj.ClockRate
			currDelta = math.Max(0, currDelta)
		}

		if sliderLast, ok := osuLastLastObj.BaseObject.(*preprocessing.LazySlider); ok {
			lastDelta -= sliderLast.GetDuration() / osuLastObj.ClockRate
			lastDelta = math.Max(0, lastDelta)
		}

		rhythmChangeBonus = getRhythmDifference(currDelta, lastDelta)
	}

	// Combine all factors
	result := velocityChangePart*velocityChangeBonus + angleChangePart*angleChangeBonus + rhythmChangePart*rhythmChangeBonus
	return result
}

func getVelocityChangeFactor(osuCurrObj, osuLastObj *preprocessing.DifficultyObject) float64 {
	currVelocity := osuCurrObj.LazyJumpDistance / osuCurrObj.StrainTime
	prevVelocity := osuLastObj.LazyJumpDistance / osuLastObj.StrainTime

	var velocityChangeFactor float64

	// Check if currVelocity or prevVelocity is greater than 0
	if currVelocity > 0 || prevVelocity > 0 {
		// Calculate velocityChange
		velocityChange := math.Max(0, math.Min(
			math.Abs(prevVelocity-currVelocity)-0.5*math.Min(currVelocity, prevVelocity),
			math.Max(osuCurrObj.Diff.CircleRadiusU/math.Max(osuCurrObj.StrainTime, osuLastObj.StrainTime), math.Min(currVelocity, prevVelocity)),
		))

		// Calculate velocityChangeFactor
		velocityChangeFactor = velocityChange / math.Max(currVelocity, prevVelocity)
		velocityChangeFactor /= 0.4 // max is 0.4
	}

	return velocityChangeFactor
}

func getTimeNerfFactor(deltaTime float64) float64 {
	return mutils.Clamp(2.0-deltaTime/(reading_window_size/2), 0.0, 1.0)
}

func getRhythmDifference(t1, t2 float64) float64 {
	return 1 - math.Min(t1, t2)/math.Max(t1, t2)
}

func logistic(x float64) float64 {
	return 1 / (1 + math.Exp(-x))
}

func boundBinarySearch(arr []preprocessing.ReadingObject, target float64) float64 {
	low := 0
	high := len(arr)
	result := -1

	for low < high {
		mid := low + (high-low)/2

		if arr[mid].HitObject.StartTime >= target {
			result = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	if result == -1 {
		return 0
	}
	return arr[result].Overlapness
}
