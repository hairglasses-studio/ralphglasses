package session

import (
	"testing"
)

func TestMaxPriority_Empty(t *testing.T) {
	got := maxPriority(nil)
	if got != 0 {
		t.Errorf("maxPriority(nil) = %d, want 0", got)
	}
}

func TestMaxPriority_SingleItem(t *testing.T) {
	reqs := []BatchRequest{{Priority: 5}}
	got := maxPriority(reqs)
	if got != 5 {
		t.Errorf("maxPriority([5]) = %d, want 5", got)
	}
}

func TestMaxPriority_MultipleItems(t *testing.T) {
	reqs := []BatchRequest{
		{Priority: 3},
		{Priority: 10},
		{Priority: 7},
		{Priority: 1},
	}
	got := maxPriority(reqs)
	if got != 10 {
		t.Errorf("maxPriority([3,10,7,1]) = %d, want 10", got)
	}
}

func TestMaxPriority_AllZero(t *testing.T) {
	reqs := []BatchRequest{
		{Priority: 0},
		{Priority: 0},
	}
	got := maxPriority(reqs)
	if got != 0 {
		t.Errorf("maxPriority([0,0]) = %d, want 0", got)
	}
}
