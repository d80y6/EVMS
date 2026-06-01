package main

import (
	"testing"
)

func TestCrossProduct(t *testing.T) {
	cp := crossProduct(Point{0, 0}, Point{10, 0}, Point{5, 5})
	if cp <= 0 {
		t.Fatalf("expected positive cross product, got %f", cp)
	}
}

func TestLineIntersect(t *testing.T) {
	if !lineIntersect(Point{0, 0}, Point{10, 10}, Point{0, 10}, Point{10, 0}) {
		t.Fatal("expected lines to intersect")
	}
	if lineIntersect(Point{0, 0}, Point{10, 0}, Point{0, 5}, Point{10, 5}) {
		t.Fatal("expected parallel lines not to intersect")
	}
}

func TestTripwireCrossing(t *testing.T) {
	tw := Tripwire{
		Start: Point{0, 5},
		End:   Point{10, 5},
	}
	if !tw.CheckCrossing(Point{5, 0}, Point{5, 10}) {
		t.Fatal("expected crossing detection")
	}
	if tw.CheckCrossing(Point{5, 0}, Point{5, 4}) {
		t.Fatal("expected no crossing")
	}
}

func TestDirectionFilter(t *testing.T) {
	tw := Tripwire{Direction: "left_to_right"}
	if !tw.CheckDirection(Point{0, 5}, Point{10, 5}) {
		t.Fatal("expected left_to_right to match")
	}
	if tw.CheckDirection(Point{10, 5}, Point{0, 5}) {
		t.Fatal("expected right_to_left not to match left_to_right")
	}
}
