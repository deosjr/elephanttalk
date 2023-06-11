package talk

import (
	"image"
	"image/color"
	"math/rand"
	"time"

	"gocv.io/x/gocv"
)

// TODO: call from examples folder?
func calibrationPage() {
	w, h := 2480, 3508 // 300 ppi/dpi
	// a4 in cm: 21 x 29.7
	// which means 1cm in pixels = 2480/21 =~ 118
	img := gocv.NewMatWithSize(h, w, gocv.MatTypeCV8UC3)
	defer img.Close()

	red := cielabRed
	green := cielabGreen
	blue := cielabBlue
	yellow := cielabYellow
	white := color.RGBA{255, 255, 255, 0}

	midw, midh := w/2., h/2.
	d := int(1.5 * 118) // circle radius = 1, circle distance = 1
	gocv.Rectangle(&img, image.Rect(0, 0, w, h), white, -1)
	gocv.Circle(&img, image.Pt(midw-d, midh-d), 1*118, red, -1)
	gocv.Circle(&img, image.Pt(midw+d, midh-d), 1*118, green, -1)
	gocv.Circle(&img, image.Pt(midw-d, midh+d), 1*118, blue, -1)
	gocv.Circle(&img, image.Pt(midw+d, midh+d), 1*118, yellow, -1)

	gocv.IMWrite("out.png", img)
}

// TODO: takes a page as input, move page generation out
func blank() {
	w, h := 2480, 3508 // 300 ppi/dpi
	// a4 in cm: 21 x 29.7
	// which means 1cm in pixels = 2480/21 =~ 118
	img := gocv.NewMatWithSize(h, w, gocv.MatTypeCV8UC3)
	defer img.Close()

	red := cielabRed
	green := cielabGreen
	blue := cielabBlue
	yellow := cielabYellow
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

	gocv.IMWrite("out.png", img)
}
