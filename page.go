package main

import (
	"image"
	"image/color"
	"math/rand"
	"sort"
	"time"

	"github.com/deosjr/lispadventures/lisp"
	"gocv.io/x/gocv"
)

type page struct {
	id   int
	code lisp.SExpression
}

type corner struct {
	ll, l, m, r, rr dot
}

type dot struct {
	x, y int
	c    dotColor
}

type dotColor uint8

const (
	unrecognized dotColor = iota
	redDot
	blueDot
	greenDot
	yellowDot
)

func equalWithMargin(x, y, margin float64) bool {
	return !(x-margin > y || x+margin < y)
}

func findCorners(v []circle) ([]circle, bool) {
	if len(v) != 5 {
		return nil, false
	}
	midpoint := v[0].mid
	for _, p := range v[1:] {
		midpoint = midpoint.Add(p.mid)
	}
	midpoint = midpoint.Div(len(v))

	sortedDistances := []float64{}
	for _, p := range v {
		sortedDistances = append(sortedDistances, euclidian(midpoint.Sub(p.mid)))
	}
	sort.Float64s(sortedDistances)
	// first 3 are roughly equal, last 2 are roughly x2
	// middle one is the 'top' of the 'arrow'
	first3 := (sortedDistances[0] + sortedDistances[1] + sortedDistances[2]) / 3.0
	last2 := (sortedDistances[3] + sortedDistances[4]) / 2.0
	if !equalWithMargin(first3*2, last2, 5.0) {
		return nil, false
	}
	if !equalWithMargin(sortedDistances[0], sortedDistances[1], 3.0) {
		return nil, false
	}
	if !equalWithMargin(sortedDistances[0], sortedDistances[2], 6.0) {
		return nil, false
	}
	if !equalWithMargin(sortedDistances[1], sortedDistances[2], 6.0) {
		return nil, false
	}
	if !equalWithMargin(sortedDistances[3], sortedDistances[4], 6.0) {
		return nil, false
	}

	sort.Slice(v, func(i, j int) bool {
		return euclidian(midpoint.Sub(v[i].mid)) < euclidian(midpoint.Sub(v[j].mid))
	})

	return v, true
}

func calibrationPage() {
	w, h := 2480, 3508 // 300 ppi/dpi
	// a4 in cm: 21 x 29.7
	// which means 1cm in pixels = 2480/21 =~ 118
	img := gocv.NewMatWithSize(h, w, gocv.MatTypeCV8UC3)
	defer img.Close()

	red := color.RGBA{255, 0, 0, 0}
	green := color.RGBA{0, 255, 0, 0}
	blue := color.RGBA{0, 0, 255, 0}
	yellow := color.RGBA{255, 255, 0, 0}
	white := color.RGBA{255, 255, 255, 0}

	midw, midh := w/2., h/2.
	d := int(1.5 * 118) // circle radius = 1, circle distance = 1
	gocv.Rectangle(&img, image.Rect(0, 0, w, h), white, -1)
	gocv.Circle(&img, image.Pt(midw-d, midh-d), 1*118, red, -1)
	gocv.Circle(&img, image.Pt(midw+d, midh-d), 1*118, green, -1)
	gocv.Circle(&img, image.Pt(midw-d, midh+d), 1*118, blue, -1)
	gocv.Circle(&img, image.Pt(midw+d, midh+d), 1*118, yellow, -1)

	// TODO: use CIE LAB color space?
	//gocv.CvtColor(img, &img, gocv.ColorRGBToLab)

	gocv.IMWrite("out.png", img)
}

func blank() {
	w, h := 2480, 3508 // 300 ppi/dpi
	// a4 in cm: 21 x 29.7
	// which means 1cm in pixels = 2480/21 =~ 118
	img := gocv.NewMatWithSize(h, w, gocv.MatTypeCV8UC3)
	defer img.Close()

	red := color.RGBA{255, 0, 0, 0}
	green := color.RGBA{0, 255, 0, 0}
	blue := color.RGBA{0, 0, 255, 0}
	yellow := color.RGBA{255, 255, 0, 0}
	white := color.RGBA{255, 255, 255, 0}

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	colors := []color.RGBA{red, green, blue, yellow}
	randomColor := func() color.RGBA {
		return colors[rnd.Intn(4)]
	}

	r := 118
	d := r / 2

	gocv.Rectangle(&img, image.Rect(0, 0, w, h), white, -1)

	gocv.Circle(&img, image.Pt(d+r, d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(2*d+3*r, d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(3*d+5*r, d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(d+r, 2*d+3*r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(d+r, 3*d+5*r), r, randomColor(), -1)

	gocv.Circle(&img, image.Pt(w-(d+r), d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(2*d+3*r), d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(3*d+5*r), d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(d+r), 2*d+3*r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(d+r), 3*d+5*r), r, randomColor(), -1)

	gocv.Circle(&img, image.Pt(d+r, h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(2*d+3*r, h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(3*d+5*r, h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(d+r, h-(2*d+3*r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(d+r, h-(3*d+5*r)), r, randomColor(), -1)

	gocv.Circle(&img, image.Pt(w-(d+r), h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(2*d+3*r), h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(3*d+5*r), h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(d+r), h-(2*d+3*r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(d+r), h-(3*d+5*r)), r, randomColor(), -1)

	// TODO: use CIE LAB color space?
	//gocv.CvtColor(img, &img, gocv.ColorRGBToLab)

	gocv.IMWrite("out.png", img)
}
