package play

import (
	"github.com/wieku/danser-go/app/beatmap"
	"github.com/wieku/danser-go/app/osuapi"
	"github.com/wieku/danser-go/app/settings"
	"github.com/wieku/danser-go/app/skin"
	"github.com/wieku/danser-go/framework/graphics/batch"
	"github.com/wieku/danser-go/framework/graphics/sprite"
	"github.com/wieku/danser-go/framework/math/animation"
	"github.com/wieku/danser-go/framework/math/animation/easing"
	"github.com/wieku/danser-go/framework/math/vector"
	"log"
	"sort"
)

const spacing = 57.6
const padding = 142.0
const start = 348.0
const visible = 6

type ScoreBoard struct {
	scores        []*ScoreboardEntry
	displayScores []*ScoreboardEntry

	time float64

	playerIndex     int
	lastPlayerIndex int
	playerEntry     *ScoreboardEntry

	explosionManager *sprite.Manager
	first            bool
	avatarsVisible   bool

	currentPlayerURL string

	width float64
}

func NewScoreboard(beatMap *beatmap.BeatMap, omitID int64) *ScoreBoard {
	board := &ScoreBoard{
		first:            true,
		explosionManager: sprite.NewManager(),
		width:            768 * settings.Graphics.GetAspectRatio(),
	}

	skin.GetTextureSource("scoreboard-explosion-1", skin.LOCAL)
	skin.GetTextureSource("scoreboard-explosion-2", skin.LOCAL)

	if settings.Gameplay.ScoreBoard.HideOthers {
		return board
	}

	var mods []string

	if settings.Gameplay.ScoreBoard.ModsOnly {
		for _, mInfo := range beatMap.Diff.ExportMods2() {
			mods = append(mods, mInfo.Acronym)
		}
	}

	scores, err := osuapi.GetScoresCheksum(beatMap.MD5, true, osuapi.NormalMode, 51, mods...)

	if err != nil {
		log.Println("Error connecting to osu!api:", err)
	} else if len(scores) == 0 {
		log.Println("Can't find online scores!")
	} else {
		for i := 0; i < len(scores); i++ {
			if scores[i].LegacyScoreID == omitID || scores[i].ID == omitID {
				board.currentPlayerURL = scores[i].User.AvatarURL
				scores = append(scores[:i], scores[i+1:]...)
				i--
			}
		}

		sort.SliceStable(scores, func(i, j int) bool {
			return scores[i].LegacyTotalScore > scores[j].LegacyTotalScore
		})

		for i := 0; i < min(len(scores), 50); i++ {
			s := scores[i]

			entry := NewScoreboardEntry(s.User.Username, int64(s.LegacyTotalScore), int64(s.MaxCombo), i+1, false)

			if settings.Gameplay.ScoreBoard.ShowAvatars {
				entry.LoadAvatarURL(s.User.AvatarURL)
			}

			board.scores = append(board.scores, entry)
			board.displayScores = append(board.displayScores, entry)
		}

		log.Println("SCORES", len(scores))
	}

	return board
}

func (board *ScoreBoard) AddPlayer(name string, autoPlay bool) {
	board.playerEntry = NewScoreboardEntry(name, 0, 0, len(board.scores)+1, true)
	board.playerIndex = len(board.scores)
	board.lastPlayerIndex = board.playerIndex

	board.scores = append(board.scores, board.playerEntry)
	board.displayScores = append(board.displayScores, board.playerEntry)

	if settings.Gameplay.ScoreBoard.ShowAvatars {
		if autoPlay {
			board.playerEntry.LoadDefaultAvatar()
		} else {
			if board.currentPlayerURL != "" {
				board.playerEntry.LoadAvatarURL(board.currentPlayerURL)
			} else {
				board.playerEntry.LoadAvatarUser(name)
			}
		}
	}

	board.UpdatePlayer(0, 0)

	hasAvatar := false

	for _, e := range board.scores {
		if e.IsAvatarLoaded() {
			hasAvatar = true
			break
		}
	}

	if hasAvatar {
		for _, e := range board.scores {
			e.ShowAvatar(true)
		}
	}

	board.avatarsVisible = hasAvatar
}

func (board *ScoreBoard) UpdatePlayer(score, combo int64) {
	board.playerEntry.score = score
	board.playerEntry.combo = combo

	sort.SliceStable(board.scores, func(i, j int) bool {
		return board.scores[i].score > board.scores[j].score
	})

	for i := 0; i < len(board.scores); i++ {
		entry := board.scores[i]

		entry.rank = i + 1

		entry.UpdateData()

		if entry == board.playerEntry {
			board.playerIndex = i
			if board.playerIndex < board.lastPlayerIndex {
				playerPos := board.playerEntry.GetPosition()

				align := vector.CentreLeft

				if settings.Gameplay.ScoreBoard.AlignRight {
					align = vector.CentreRight
				}

				sprite2 := sprite.NewSpriteSingle(skin.GetTexture("scoreboard-explosion-2"), 0.5, playerPos, align)
				sprite2.AddTransform(animation.NewSingleTransform(animation.Fade, easing.Linear, board.time, board.time+400, 1, 0))
				sprite2.AddTransform(animation.NewVectorTransform(animation.ScaleVector, easing.OutQuad, board.time, board.time+200, 1, 1, max(1, 16*settings.Gameplay.ScoreBoard.ExplosionScale), 1.2))
				sprite2.ResetValuesToTransforms()
				sprite2.AdjustTimesToTransformations()
				sprite2.ShowForever(false)

				sprite1 := sprite.NewSpriteSingle(skin.GetTexture("scoreboard-explosion-1"), 1, playerPos, align)
				sprite1.AddTransform(animation.NewSingleTransform(animation.Fade, easing.Linear, board.time, board.time+700, 1, 0))
				sprite1.AddTransform(animation.NewVectorTransform(animation.ScaleVector, easing.OutQuad, board.time, board.time+700, 1, 1, 1, 1.3))
				sprite1.ResetValuesToTransforms()
				sprite1.AdjustTimesToTransformations()
				sprite1.ShowForever(false)

				if settings.Gameplay.ScoreBoard.AlignRight {
					sprite2.SetHFlip(true)
					sprite1.SetHFlip(true)
				}

				board.explosionManager.Add(sprite2)
				board.explosionManager.Add(sprite1)
			}

			board.lastPlayerIndex = board.playerIndex
		}
	}

	startI := 0
	if board.playerIndex > visible-1 {
		startI = board.playerIndex - (visible - 1)
	}

	shiftI := 0

	for i := 0; i < len(board.scores); i++ {
		entry := board.scores[i]

		display := i == 0
		if i > startI && shiftI < visible {
			display = true
		}

		pX := settings.Gameplay.ScoreBoard.XOffset
		if settings.Gameplay.ScoreBoard.AlignRight {
			pX += board.width
		}

		target := vector.NewVec2d(pX, start+settings.Gameplay.ScoreBoard.YOffset+float64(shiftI)*spacing*settings.Gameplay.ScoreBoard.Scale)

		if board.first {
			entry.SetPosition(target)
		} else {
			entry.ClearTransformationsOfType(animation.Move)
			entry.AddTransform(animation.NewVectorTransformV(animation.Move, easing.OutQuad, board.time, board.time+600, entry.GetPosition(), target))
		}

		alpha := 0.8
		if i != board.playerIndex {
			alpha -= 0.3 * float64(i) / float64(len(board.scores))
		}

		if display {
			entry.AddTransform(animation.NewSingleTransform(animation.Fade, easing.Linear, board.time, board.time+600, entry.GetAlpha(), alpha))

			shiftI++
		} else if entry.visible {
			entry.AddTransform(animation.NewSingleTransform(animation.Fade, easing.Linear, board.time, board.time+600, entry.GetAlpha(), 0))
		}

		entry.visible = display
	}

	board.first = false
}

func (board *ScoreBoard) Update(time float64) {
	board.time = time

	board.explosionManager.Update(time)

	for _, e := range board.scores {
		e.Update(time)
	}
}

func (board *ScoreBoard) Draw(batch *batch.QuadBatch, alpha float64) {
	if !settings.Gameplay.ScoreBoard.Show {
		return
	}

	alpha *= settings.Gameplay.ScoreBoard.Opacity

	for _, e := range board.displayScores {
		e.Draw(board.time, batch, alpha)
	}

	scale := settings.Gameplay.ScoreBoard.Scale
	batch.SetScale(scale, scale)
	board.explosionManager.Draw(board.time, batch)
	batch.SetScale(1, 1)
}
