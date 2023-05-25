package main

import (
	"image"
	"image/color"
	"math"
	"sort"
)

type circle struct {
	mid image.Point
	r   int
	c   color.Color
}

func euclidian(p image.Point) float64 {
	return math.Sqrt(float64(p.X*p.X + p.Y*p.Y))
}

func translate(p, delta image.Point, ratio float64) image.Point {
	// first we add the difference from webcam to beamer midpoints
	q := p.Add(delta)
	// then we boost from midpoint by missing ratio
	beamerMid := image.Pt(beamerWidth/2., beamerHeight/2.)
	deltaV := q.Sub(beamerMid)
	adjust := image.Pt(int(float64(deltaV.X)*((1./ratio)-1)), int(float64(deltaV.Y)*((1./ratio)-1)))
	return q.Add(adjust)
}

// counterclockwise rotation
// TODO: ???? expected counterclockwise but getting clockwise ????
// 'fixed' by flipping sign on angle in sin/cos, shouldnt be there
func rotateAround(pivot, point image.Point, radians float64) image.Point {
	s := math.Sin(-radians)
	c := math.Cos(-radians)

	x := float64(point.X - pivot.X)
	y := float64(point.Y - pivot.Y)

	xNew := (c*x - s*y) + float64(pivot.X)
	yNew := (s*x + c*y) + float64(pivot.Y)
	return image.Pt(int(xNew), int(yNew))
}

func angleBetween(u, v image.Point) float64 {
	dot := float64(u.X*v.X + u.Y*v.Y)
	return math.Acos(dot / (euclidian(u) * euclidian(v)))
}

func sortCirclesAsCorners(circles []circle) {
	// ulhc, urhc, llhc, lrhc
	sort.Slice(circles, func(i, j int) bool {
		return circles[i].mid.X+circles[i].mid.Y < circles[j].mid.X+circles[j].mid.Y
	})
	// since origin is upperleft, ulhc is first and lrhc is last
	// urhc and llhc is unordered yet; urhc is assumed to be higher up
	if circles[1].mid.Y > circles[2].mid.Y {
		circles[1], circles[2] = circles[2], circles[1]
	}
}

// same thing
func sortCorners(corners []corner) {
	sort.Slice(corners, func(i, j int) bool {
		return corners[i].m.p.X+corners[i].m.p.Y < corners[j].m.p.X+corners[j].m.p.Y
	})
	if corners[1].m.p.Y > corners[2].m.p.Y {
		corners[1], corners[2] = corners[2], corners[1]
	}
}

func circlesMidpoint(circles []circle) image.Point {
	mid := circles[0].mid
	for _, c := range circles[1:] {
		mid = mid.Add(c.mid)
	}
	return mid.Div(len(circles))
}

func ptsToRect(pts []image.Point) image.Rectangle {
	r := image.Rectangle{
		pts[0].Add(image.Pt(-1, -1)),
		pts[0].Add(image.Pt(1, 1)),
	}
	for _, p := range pts {
		r = r.Union(image.Rectangle{
			p.Add(image.Pt(-1, -1)),
			p.Add(image.Pt(1, 1)),
		})
	}
	return r
}

// calculate diff with reference color naively as a euclidian distance in color space
func colorDistance(sample, reference color.Color) float64 {
	rr, gg, bb, _ := sample.RGBA()
	refR, refG, refB, _ := reference.RGBA()
	dR := float64(rr) - float64(refR)
	dG := float64(gg) - float64(refG)
	dB := float64(bb) - float64(refB)
	return math.Sqrt(dR*dR + dG*dG + dB*dB)
}
