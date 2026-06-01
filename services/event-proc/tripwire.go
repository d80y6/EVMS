package main

import (
	"math"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Tripwire struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Start     Point   `json:"start"`
	End       Point   `json:"end"`
	Direction string  `json:"direction"`
	CameraID  string  `json:"camera_id"`
}

type TrackPosition struct {
	TrackID    string
	PrevCenter Point
	CurrCenter Point
}

func crossProduct(lineStart, lineEnd, point Point) float64 {
	return (lineEnd.X-lineStart.X)*(point.Y-lineStart.Y) - (lineEnd.Y-lineStart.Y)*(point.X-lineStart.X)
}

func lineIntersect(p1, p2, q1, q2 Point) bool {
	d1 := crossProduct(q1, q2, p1)
	d2 := crossProduct(q1, q2, p2)
	d3 := crossProduct(p1, p2, q1)
	d4 := crossProduct(p1, p2, q2)

	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	if d1 == 0 && onSegment(q1, q2, p1) {
		return true
	}
	if d2 == 0 && onSegment(q1, q2, p2) {
		return true
	}
	if d3 == 0 && onSegment(p1, p2, q1) {
		return true
	}
	if d4 == 0 && onSegment(p1, p2, q2) {
		return true
	}

	return false
}

func onSegment(a, b, c Point) bool {
	return math.Min(a.X, b.X) <= c.X && c.X <= math.Max(a.X, b.X) &&
		math.Min(a.Y, b.Y) <= c.Y && c.Y <= math.Max(a.Y, b.Y)
}

func (t *Tripwire) CheckCrossing(prev, curr Point) bool {
	return lineIntersect(t.Start, t.End, prev, curr)
}

func (t *Tripwire) CheckDirection(prev, curr Point) bool {
	if t.Direction == "any" {
		return true
	}

	dx := curr.X - prev.X
	dy := curr.Y - prev.Y

	switch t.Direction {
	case "left_to_right":
		return dx > 0 && math.Abs(dx) > math.Abs(dy)
	case "right_to_left":
		return dx < 0 && math.Abs(dx) > math.Abs(dy)
	case "top_to_bottom":
		return dy > 0 && math.Abs(dy) > math.Abs(dx)
	case "bottom_to_top":
		return dy < 0 && math.Abs(dy) > math.Abs(dx)
	}
	return false
}
