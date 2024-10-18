package preprocessing

import (
	"math"

	"github.com/wieku/danser-go/app/beatmap/difficulty"
	"github.com/wieku/danser-go/app/beatmap/objects"
	"github.com/wieku/danser-go/framework/math/math32"
	"github.com/wieku/danser-go/framework/math/mutils"
	"github.com/wieku/danser-go/framework/math/vector"
)

const (
	NormalizedRadius        = 50.0
	CircleSizeBuffThreshold = 30.0
	MinDeltaTime            = 25
)

type ReadingObject struct {
	HitObject   *DifficultyObject
	Overlapness float64
}

type DifficultyObject struct {
	// That's stupid but oh well
	listOfDiffs *[]*DifficultyObject
	Index       int

	Diff *difficulty.Difficulty

	BaseObject objects.IHitObject

	IsSlider  bool
	IsSpinner bool

	lastObject objects.IHitObject

	lastLastObject objects.IHitObject

	DeltaTime float64

	StartTime float64

	EndTime float64

	LazyJumpDistance float64

	MinimumJumpDistance float64

	TravelDistance float64

	Angle float64

	AngleSigned float64

	MinimumJumpTime float64

	TravelTime float64

	StrainTime float64

	GreatWindow float64

	ClockRate float64

	Preempt float64

	FollowLineTime float64

	RhythmDifficulty float64

	AnglePredictability float64

	ReadingObjects []ReadingObject

	OverlapValues map[int]float64
}

func NewDifficultyObject(hitObject, lastLastObject, lastObject objects.IHitObject, d *difficulty.Difficulty, listOfDiffs *[]*DifficultyObject, index int) *DifficultyObject {
	obj := &DifficultyObject{
		listOfDiffs:      listOfDiffs,
		Index:            index,
		Diff:             d,
		BaseObject:       hitObject,
		lastObject:       lastObject,
		lastLastObject:   lastLastObject,
		DeltaTime:        (hitObject.GetStartTime() - lastObject.GetStartTime()) / d.Speed,
		StartTime:        hitObject.GetStartTime() / d.Speed,
		EndTime:          hitObject.GetEndTime() / d.Speed,
		Angle:            math.NaN(),
		AngleSigned:      math.NaN(),
		GreatWindow:      2 * d.Hit300U / d.Speed,
		ClockRate:        d.Speed,
		Preempt:          d.PreemptU / d.Speed,
		FollowLineTime:   0,
		RhythmDifficulty: math.NaN(),
	}

	if _, ok := hitObject.(*objects.Spinner); ok {
		obj.IsSpinner = true
	}

	if _, ok := hitObject.(*LazySlider); ok {
		obj.IsSlider = true
	}

	obj.StrainTime = max(obj.DeltaTime, MinDeltaTime)

	obj.setDistances()

	if !hitObject.IsNewCombo() {
		obj.FollowLineTime = 800.0 / d.Speed
	}

	obj.AnglePredictability = obj.CalculateAnglePredictability()
	obj.ReadingObjects = obj.getReadingObjects()

	return obj
}

func (o *DifficultyObject) GetDoubletapness(osuNextObj *DifficultyObject) float64 {
	if osuNextObj != nil {
		currDeltaTime := max(1, o.DeltaTime)
		nextDeltaTime := max(1, osuNextObj.DeltaTime)
		deltaDifference := math.Abs(nextDeltaTime - currDeltaTime)
		speedRatio := currDeltaTime / max(currDeltaTime, deltaDifference)
		windowRatio := math.Pow(min(1, currDeltaTime/o.GreatWindow), 2)
		return 1 - math.Pow(speedRatio, 1-windowRatio)
	}

	return 0
}

func (o *DifficultyObject) OpacityAt(time float64) float64 {
	if time > o.BaseObject.GetStartTime() {
		return 0
	}

	fadeInStartTime := o.BaseObject.GetStartTime() - o.Diff.PreemptU
	fadeInDuration := o.Diff.TimeFadeIn

	if o.Diff.CheckModActive(difficulty.Hidden) {
		fadeOutStartTime := o.BaseObject.GetStartTime() - o.Diff.PreemptU + o.Diff.TimeFadeIn
		fadeOutDuration := o.Diff.PreemptU * 0.3

		return min(
			mutils.Clamp((time-fadeInStartTime)/fadeInDuration, 0.0, 1.0),
			1.0-mutils.Clamp((time-fadeOutStartTime)/fadeOutDuration, 0.0, 1.0),
		)
	}

	return mutils.Clamp((time-fadeInStartTime)/fadeInDuration, 0.0, 1.0)
}

func (o *DifficultyObject) Previous(backwardsIndex int) *DifficultyObject {
	index := o.Index - (backwardsIndex + 1)

	if index < 0 {
		return nil
	}

	return (*o.listOfDiffs)[index]
}

func (o *DifficultyObject) Next(forwardsIndex int) *DifficultyObject {
	index := o.Index + (forwardsIndex + 1)

	if index >= len(*o.listOfDiffs) {
		return nil
	}

	return (*o.listOfDiffs)[index]
}

func (o *DifficultyObject) setDistances() {
	if currentSlider, ok := o.BaseObject.(*LazySlider); ok {
		// danser's RepeatCount considers first span, that's why we have to subtract 1 here
		o.TravelDistance = float64(currentSlider.LazyTravelDistance * float32(math.Pow(1+float64(currentSlider.RepeatCount-1)/2.5, 1.0/2.5)))
		o.TravelTime = max(currentSlider.LazyTravelTime/o.Diff.Speed, MinDeltaTime)
	}

	_, ok1 := o.BaseObject.(*objects.Spinner)
	_, ok2 := o.lastObject.(*objects.Spinner)

	if ok1 || ok2 {
		return
	}

	scalingFactor := NormalizedRadius / float32(o.Diff.CircleRadiusU)

	if o.Diff.CircleRadiusU < CircleSizeBuffThreshold {
		smallCircleBonus := min(CircleSizeBuffThreshold-float32(o.Diff.CircleRadiusU), 5.0) / 50.0
		scalingFactor *= 1.0 + smallCircleBonus
	}

	lastCursorPosition := getEndCursorPosition(o.lastObject, o.Diff)

	o.LazyJumpDistance = float64((o.BaseObject.GetStackedStartPositionMod(o.Diff.Mods).Scl(scalingFactor)).Dst(lastCursorPosition.Scl(scalingFactor)))
	o.MinimumJumpTime = o.StrainTime
	o.MinimumJumpDistance = o.LazyJumpDistance

	if lastSlider, ok := o.lastObject.(*LazySlider); ok {
		lastTravelTime := max(lastSlider.LazyTravelTime/o.Diff.Speed, MinDeltaTime)
		o.MinimumJumpTime = max(o.StrainTime-lastTravelTime, MinDeltaTime)

		//
		// There are two types of slider-to-object patterns to consider in order to better approximate the real movement a player will take to jump between the hitobjects.
		//
		// 1. The anti-flow pattern, where players cut the slider short in order to move to the next hitobject.
		//
		//      <======o==>  ← slider
		//             |     ← most natural jump path
		//             o     ← a follow-up hitcircle
		//
		// In this case the most natural jump path is approximated by LazyJumpDistance.
		//
		// 2. The flow pattern, where players follow through the slider to its visual extent into the next hitobject.
		//
		//      <======o==>---o
		//                  ↑
		//        most natural jump path
		//
		// In this case the most natural jump path is better approximated by a new distance called "tailJumpDistance" - the distance between the slider's tail and the next hitobject.
		//
		// Thus, the player is assumed to jump the minimum of these two distances in all cases.
		//

		tailJumpDistance := lastSlider.GetStackedPositionAtModLazer(lastSlider.EndTimeLazer, o.Diff.Mods).Dst(o.BaseObject.GetStackedStartPositionMod(o.Diff.Mods)) * scalingFactor
		o.MinimumJumpDistance = max(0, min(o.LazyJumpDistance-float64(maximumSliderRadius-assumedSliderRadius), float64(tailJumpDistance-maximumSliderRadius)))
	}

	if o.lastLastObject != nil {
		if _, ok := o.lastLastObject.(*objects.Spinner); ok {
			return
		}

		lastLastCursorPosition := getEndCursorPosition(o.lastLastObject, o.Diff)

		v1 := lastLastCursorPosition.Sub(o.lastObject.GetStackedStartPositionMod(o.Diff.Mods))
		v2 := o.BaseObject.GetStackedStartPositionMod(o.Diff.Mods).Sub(lastCursorPosition)
		dot := v1.Dot(v2)
		det := v1.X*v2.Y - v1.Y*v2.X
		o.AngleSigned = float64(math32.Atan2(det, dot))
		o.Angle = math.Abs(o.AngleSigned)
	}
}

func getEndCursorPosition(obj objects.IHitObject, d *difficulty.Difficulty) (pos vector.Vector2f) {
	pos = obj.GetStackedStartPositionMod(d.Mods)

	if s, ok := obj.(*LazySlider); ok {
		pos = s.LazyEndPosition
	}

	return
}

func (o *DifficultyObject) getOpacityMultiplier(loopObj *DifficultyObject) float64 {
	const threshold = 0.3

	// Get raw opacity
	opacity := o.OpacityAt(loopObj.BaseObject.GetStartTime())

	opacity = math.Min(1, opacity+threshold) // object with opacity 0.7 are still perfectly visible
	opacity -= threshold                     // return opacity 0 objects back to 0
	opacity /= 1 - threshold                 // fix scaling to be 0-1 again
	opacity = math.Sqrt(opacity)             // change curve

	return opacity
}

func getTimeDifference(timeA, timeB float64) float64 {
	similarity := math.Min(timeA, timeB) / math.Max(timeA, timeB)
	if math.Max(timeA, timeB) == 0 {
		similarity = 1
	}

	if similarity < 0.75 {
		return 1.0
	}
	if similarity > 0.9 {
		return 0.0
	}

	return (math.Cos((similarity-0.75)*math.Pi/0.15) + 1) / 2 // drops from 1 to 0 as similarity increases from 0.75 to 0.9
}

func getAngleSimilarity(angle1, angle2 float64) float64 {
	difference := math.Abs(angle1 - angle2)
	threshold := math.Pi / 12

	if difference > threshold {
		return 0
	}
	return 1 - difference/threshold
}

func calculateOverlapness(odho1, odho2 *DifficultyObject) float64 {
	const areaCoef = 0.85
	const stackDistanceRatio = 0.1414213562373

	distance := float64(odho1.BaseObject.GetStackedStartPosition().Dst(odho2.BaseObject.GetStackedStartPosition()))
	radius := odho1.Diff.CircleRadiusU

	distanceSqr := distance * distance
	radiusSqr := radius * radius

	if distance > radius*2 {
		return 0
	}

	s1 := math.Acos(distance/(2*radius)) * radiusSqr        // Area of sector
	s2 := distance * math.Sqrt(radiusSqr-distanceSqr/4) / 2 // Area of triangle

	overlappingAreaNormalized := (s1 - s2) * 2 / (math.Pi * radiusSqr)

	perfectStackBuff := (stackDistanceRatio - distance/radius) / stackDistanceRatio // scale from 0 on normal stack to 1 on perfect stack
	perfectStackBuff = math.Max(perfectStackBuff, 0)                                // can't be negative

	return overlappingAreaNormalized*areaCoef + perfectStackBuff*(1-areaCoef)
}

func retrieveCurrentVisibleObjects(current *DifficultyObject) []*DifficultyObject {
	visibleObjects := []*DifficultyObject{}

	for i := 0; i < current.Index+1; i++ {
		hitObject := current.Previous(i)

		if hitObject == nil || hitObject.StartTime < current.StartTime-current.Preempt {
			break
		}

		visibleObjects = append(visibleObjects, hitObject)
	}

	return visibleObjects
}

func getGeneralSimilarity(o1, o2 *DifficultyObject) float64 {
	if o1 == nil || o2 == nil {
		return 1.0
	}

	if math.IsNaN(o1.AngleSigned) || math.IsNaN(o2.AngleSigned) {
		if o1.AngleSigned == o2.AngleSigned {
			return 1.0
		}
		return 0.0
	}

	timeSimilarity := 1 - getTimeDifference(o1.StrainTime, o2.StrainTime)

	angleDelta := math.Abs(o1.AngleSigned - o2.AngleSigned)
	angleDelta = mutils.Clamp(angleDelta-0.1, 0, 0.15)
	angleSimilarity := 1 - angleDelta/0.15

	distanceDelta := math.Abs(o1.LazyJumpDistance-o2.LazyJumpDistance) / NormalizedRadius
	distanceSimilarity := 1 / math.Max(1, distanceDelta)

	return timeSimilarity * angleSimilarity * distanceSimilarity
}

func (d *DifficultyObject) getReadingObjects() ([]*ReadingObject, map[int]float64) {
	totalOverlapnessDifficulty := 0.0
	currentTime := d.DeltaTime
	historicTimes := make([]float64, 0)
	historicAngles := make([]float64, 0)

	prevObject := d

	// Retrieve visible objects
	visibleObjects := retrieveCurrentVisibleObjects(d)

	readingObjects := make([]*ReadingObject, 0, len(visibleObjects))
	overlapValues := make(map[int]float64)

	for loopIndex := 0; loopIndex < len(visibleObjects); loopIndex++ {
		loopObj := visibleObjects[loopIndex]

		// Overlapness with this object
		currentOverlapness := calculateOverlapness(d, loopObj)

		// Save non-zero overlap values for future use
		if currentOverlapness > 0 {
			overlapValues[loopObj.Index] = currentOverlapness
		}

		if math.IsNaN(prevObject.Angle) {
			currentTime += prevObject.DeltaTime
			continue
		}

		// Previous angle because order is reversed
		angle := prevObject.Angle

		// Overlapness between current and previous to make streams have 0 buff
		instantOverlapness := 0.0
		if val, ok := prevObject.OverlapValues[loopObj.Index]; ok {
			instantOverlapness = val
		}

		// Nerf overlaps on wide angles
		angleFactor := 1.0
		angleFactor += (-math.Cos(angle) + 1) / 2                              // =2 for wide angles, =1 for acute angles
		instantOverlapness = math.Min(1, (0.5+instantOverlapness)*angleFactor) // wide angles are more predictable

		currentOverlapness *= (1 - instantOverlapness) * 2 // wide angles will have close-to-zero buff

		// Control overlap repetitiveness
		if currentOverlapness > 0 {
			currentOverlapness *= d.getOpacityMultiplier(loopObj) // Increase stability by using opacity

			currentMinOverlapness := currentOverlapness
			cumulativeTimeWithCurrent := currentTime

			// For every cumulative time with current
			for i := len(historicTimes) - 1; i >= 0; i-- {
				cumulativeTimeWithoutCurrent := 0.0

				// Get every possible cumulative time without current
				for j := i; j >= 0; j-- {
					cumulativeTimeWithoutCurrent += historicTimes[j]

					// Check how similar cumulative times are
					potentialMinOverlapness := currentOverlapness * getTimeDifference(cumulativeTimeWithCurrent, cumulativeTimeWithoutCurrent)
					potentialMinOverlapness *= 1 - getAngleSimilarity(angle, historicAngles[j])*(1-getTimeDifference(loopObj.StrainTime, prevObject.StrainTime))
					currentMinOverlapness = math.Min(currentMinOverlapness, potentialMinOverlapness)

					// Check how similar current time with cumulative time
					potentialMinOverlapness = currentOverlapness * getTimeDifference(currentTime, cumulativeTimeWithoutCurrent)
					potentialMinOverlapness *= 1 - getAngleSimilarity(angle, historicAngles[j])*(1-getTimeDifference(loopObj.StrainTime, prevObject.StrainTime))
					currentMinOverlapness = math.Min(currentMinOverlapness, potentialMinOverlapness)

					// Starting from this point - we will never have better match, so stop searching
					if cumulativeTimeWithoutCurrent >= cumulativeTimeWithCurrent {
						break
					}
				}
				cumulativeTimeWithCurrent += historicTimes[i]
			}

			currentOverlapness = currentMinOverlapness

			historicTimes = append(historicTimes, currentTime)
			historicAngles = append(historicAngles, angle)

			currentTime = prevObject.DeltaTime
		} else {
			currentTime += prevObject.DeltaTime
		}

		totalOverlapnessDifficulty += currentOverlapness

		newObj := &ReadingObject{
			HitObject:   loopObj,
			Overlapness: totalOverlapnessDifficulty,
		}
		readingObjects = append(readingObjects, newObj)
		prevObject = loopObj
	}

	return readingObjects, overlapValues
}

func (d *DifficultyObject) CalculateAnglePredictability() float64 {
	prevObj0 := d.Previous(0)
	prevObj1 := d.Previous(1)
	prevObj2 := d.Previous(2)

	if math.IsNaN(d.Angle) || prevObj0 == nil || math.IsNaN(prevObj0.Angle) {
		return 1.0
	}

	angleDifference := math.Abs(prevObj0.Angle - d.Angle)

	// Assume that very low spacing difference means that angles don't matter
	if prevObj0.LazyJumpDistance < NormalizedRadius {
		angleDifference *= math.Pow(prevObj0.LazyJumpDistance/NormalizedRadius, 2)
	}
	if d.LazyJumpDistance < NormalizedRadius {
		angleDifference *= math.Pow(d.LazyJumpDistance/NormalizedRadius, 2)
	}

	// Now research previous angles
	angleDifferencePrev := 0.0
	zeroAngleFactor := 1.0

	// Nerf alternating angles case
	if prevObj1 != nil && prevObj2 != nil && !math.IsNaN(prevObj1.Angle) {
		angleDifferencePrev = math.Abs(prevObj1.Angle - d.Angle)
		zeroAngleFactor = math.Pow(1-math.Min(d.Angle, prevObj0.Angle)/math.Pi, 10)
	}

	rescaleFactor := math.Pow(1-angleDifferencePrev/math.Pi, 5)

	// 0 on different rhythm, 1 on same rhythm
	rhythmFactor := 1 - getTimeDifference(d.StrainTime, prevObj0.StrainTime)

	if prevObj1 != nil {
		rhythmFactor *= 1 - getTimeDifference(prevObj0.StrainTime, prevObj1.StrainTime)
	}
	if prevObj1 != nil && prevObj2 != nil {
		rhythmFactor *= 1 - getTimeDifference(prevObj1.StrainTime, prevObj2.StrainTime)
	}

	prevAngleAdjust := math.Max(angleDifference-angleDifferencePrev, 0)
	prevAngleAdjust *= rescaleFactor
	prevAngleAdjust *= rhythmFactor
	prevAngleAdjust *= zeroAngleFactor

	angleDifference -= prevAngleAdjust

	// Explicit nerf for same pattern repeating
	prevObj3 := d.Previous(3)
	prevObj4 := d.Previous(4)
	prevObj5 := d.Previous(5)

	// 3-3 repeat
	similarity3_1 := getGeneralSimilarity(d, prevObj2)
	similarity3_2 := getGeneralSimilarity(prevObj0, prevObj3)
	similarity3_3 := getGeneralSimilarity(prevObj1, prevObj4)

	similarity3_total := similarity3_1 * similarity3_2 * similarity3_3

	// 4-4 repeat
	similarity4_1 := getGeneralSimilarity(d, prevObj3)
	similarity4_2 := getGeneralSimilarity(prevObj0, prevObj4)
	similarity4_3 := getGeneralSimilarity(prevObj1, prevObj5)

	similarity4_total := similarity4_1 * similarity4_2 * similarity4_3

	// Bandaid to fix Rubik's Cube +EZ
	wideness := 0.0
	if d.Angle > math.Pi*0.5 {
		wideness = (d.Angle/math.Pi - 0.5) * 2
		wideness = 1 - math.Pow(1-wideness, 3)
	}

	angleDifference /= 1 + wideness

	// Angle difference more than 15 degrees gets no penalty
	adjustedAngleDifference := math.Min(math.Pi/12, angleDifference)
	predictability := math.Cos(math.Min(math.Pi/2, 6*adjustedAngleDifference)) * rhythmFactor

	// Punish for big pattern similarity
	return 1 - (1-predictability)*(1-math.Max(similarity3_total, similarity4_total))
}
