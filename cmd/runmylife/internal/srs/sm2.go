// Package srs implements the SM-2 spaced repetition algorithm.
package srs

import (
	"math"
	"time"
)

// Card represents an SRS card with SM-2 scheduling state.
type Card struct {
	EasinessFactor float64   // EF, minimum 1.3
	IntervalDays   int       // current interval in days
	Repetitions    int       // consecutive correct answers
	NextReview     time.Time // when to review next
	LastReviewed   time.Time // when last reviewed
}

// NewCard creates a card with default SM-2 values.
func NewCard() Card {
	return Card{
		EasinessFactor: 2.5,
		IntervalDays:   0,
		Repetitions:    0,
		NextReview:     time.Now(),
	}
}

// ReviewResult holds the outcome of reviewing a card.
type ReviewResult struct {
	NewEF         float64
	NewInterval   int
	NewRepetitions int
	NextReview    time.Time
	WasCorrect    bool
}

// Review applies the SM-2 algorithm to a card based on quality of recall.
// Quality: 0 = complete blackout, 1 = wrong but recognized, 2 = wrong but easy to recall after seeing,
// 3 = correct with difficulty, 4 = correct after hesitation, 5 = perfect
func Review(card Card, quality int) ReviewResult {
	if quality < 0 {
		quality = 0
	}
	if quality > 5 {
		quality = 5
	}

	result := ReviewResult{
		NewEF:          card.EasinessFactor,
		NewRepetitions: card.Repetitions,
		WasCorrect:     quality >= 3,
	}

	// Update easiness factor
	q := float64(quality)
	result.NewEF = card.EasinessFactor + (0.1 - (5-q)*(0.08+(5-q)*0.02))
	if result.NewEF < 1.3 {
		result.NewEF = 1.3
	}

	if quality < 3 {
		// Reset: start over
		result.NewRepetitions = 0
		result.NewInterval = 1
	} else {
		result.NewRepetitions = card.Repetitions + 1
		switch result.NewRepetitions {
		case 1:
			result.NewInterval = 1
		case 2:
			result.NewInterval = 6
		default:
			result.NewInterval = int(math.Round(float64(card.IntervalDays) * result.NewEF))
			if result.NewInterval < 1 {
				result.NewInterval = 1
			}
		}
	}

	result.NextReview = time.Now().AddDate(0, 0, result.NewInterval)
	return result
}

// IsDue returns true if the card is due for review.
func IsDue(card Card) bool {
	return !card.NextReview.After(time.Now())
}

// DaysUntilReview returns days until next review (negative if overdue).
func DaysUntilReview(card Card) int {
	return int(time.Until(card.NextReview).Hours() / 24)
}

// DecayAlert returns true if the card's EF is dangerously low (suggesting knowledge decay).
func DecayAlert(card Card) bool {
	return card.EasinessFactor < 1.5 && card.Repetitions > 2
}
