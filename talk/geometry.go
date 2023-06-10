package talk

import (
	"image"
	"image/color"
	"math"
	"sort"
)

type point struct {
	x, y float64
}

func (p point) add(q point) point {
	return point{p.x + q.x, p.y + q.y}
}

func (p point) sub(q point) point {
	return point{p.x - q.x, p.y - q.y}
}

func (p point) div(n float64) point {
	return point{p.x / n, p.y / n}
}

func (p point) toIntPt() image.Point {
	return image.Pt(int(p.x), int(p.y))
}

type circle struct {
	mid point
	r   float64
	c   color.Color
}

func euclidian(p point) float64 {
	return math.Sqrt(p.x*p.x + p.y*p.y)
}

func translate(p, delta point, ratio float64) point {
	// first we add the difference from webcam to beamer midpoints
	q := p.add(delta)
	// then we boost from midpoint by missing ratio
	beamerMid := point{float64(beamerWidth) / 2., float64(beamerHeight) / 2.}
	deltaV := q.sub(beamerMid)
	factor := 0.
	if ratio != 0 {
		factor = (1. / ratio) - 1.
	}
	adjust := point{deltaV.x * factor, deltaV.y * factor}
	return q.add(adjust)
}

// counterclockwise rotation
// TODO: ???? expected counterclockwise but getting clockwise ????
// 'fixed' by flipping sign on angle in sin/cos, shouldnt be there
func rotateAround(pivot, p point, radians float64) point {
	s := math.Sin(-radians)
	c := math.Cos(-radians)

	x := p.x - pivot.x
	y := p.y - pivot.y

	xNew := (c*x - s*y) + pivot.x
	yNew := (s*x + c*y) + pivot.y
	return point{xNew, yNew}
}

func angleBetween(u, v point) float64 {
	dot := u.x*v.x + u.y*v.y
	return math.Acos(dot / (euclidian(u) * euclidian(v)))
}

func sortCirclesAsCorners(circles []circle) {
	// ulhc, urhc, llhc, lrhc
	sort.Slice(circles, func(i, j int) bool {
		return circles[i].mid.x+circles[i].mid.y < circles[j].mid.x+circles[j].mid.y
	})
	// since origin is upperleft, ulhc is first and lrhc is last
	// urhc and llhc is unordered yet; urhc is assumed to be higher up
	if circles[1].mid.y > circles[2].mid.y {
		circles[1], circles[2] = circles[2], circles[1]
	}
}

func circlesMidpoint(circles []circle) point {
	mid := circles[0].mid
	for _, c := range circles[1:] {
		mid = mid.add(c.mid)
	}
	return mid.div(float64(len(circles)))
}

func ptsToRect(pts []point) image.Rectangle {
	r := image.Rectangle{
		pts[0].add(point{-1, -1}).toIntPt(),
		pts[0].add(point{1, 1}).toIntPt(),
	}
	for _, p := range pts {
		r = r.Union(image.Rectangle{
			p.add(point{-1, -1}).toIntPt(),
			p.add(point{1, 1}).toIntPt(),
		})
	}
	return r
}

// calculate diff with reference color naively as a euclidian distance in color space
func colorDistance(sample, reference color.Color) float64 {
	rr, gg, bb, _ := sample.RGBA()
	refR, refG, refB, _ := reference.RGBA()
	dR := float64(rr>>8) - float64(refR>>8)
	dG := float64(gg>>8) - float64(refG>>8)
	dB := float64(bb>>8) - float64(refB>>8)
	return dR*dR + dG*dG + dB*dB
}
