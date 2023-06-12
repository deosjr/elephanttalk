package talk

import (
	"image"
	"image/color"

	"gocv.io/x/gocv"
)

// TODO: call from examples folder?
func PrintCalibrationPage() {
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

func PrintPageFromShorthand(ulhc, urhc, lrhc, llhc, code string) {
	PrintPage(page{
		ulhc: cornerShorthand(ulhc),
		urhc: cornerShorthand(urhc),
		llhc: cornerShorthand(llhc),
		lrhc: cornerShorthand(lrhc),
		code: code,
	})
}

func PrintPage(p page) {
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

	colors := []color.RGBA{red, green, blue, yellow}

	r := 118
	d := r / 2

	gocv.Rectangle(&img, image.Rect(0, 0, w, h), white, -1)

	gocv.Circle(&img, image.Pt(d+r, 3*d+5*r), r, colors[int(p.ulhc.ll.c)], -1)
	gocv.Circle(&img, image.Pt(d+r, 2*d+3*r), r, colors[int(p.ulhc.l.c)], -1)
	gocv.Circle(&img, image.Pt(d+r, d+r), r, colors[int(p.ulhc.m.c)], -1)
	gocv.Circle(&img, image.Pt(2*d+3*r, d+r), r, colors[int(p.ulhc.r.c)], -1)
	gocv.Circle(&img, image.Pt(3*d+5*r, d+r), r, colors[int(p.ulhc.rr.c)], -1)

	gocv.Circle(&img, image.Pt(w-(3*d+5*r), d+r), r, colors[int(p.urhc.ll.c)], -1)
	gocv.Circle(&img, image.Pt(w-(2*d+3*r), d+r), r, colors[int(p.urhc.l.c)], -1)
	gocv.Circle(&img, image.Pt(w-(d+r), d+r), r, colors[int(p.urhc.m.c)], -1)
	gocv.Circle(&img, image.Pt(w-(d+r), 2*d+3*r), r, colors[int(p.urhc.r.c)], -1)
	gocv.Circle(&img, image.Pt(w-(d+r), 3*d+5*r), r, colors[int(p.urhc.rr.c)], -1)

	gocv.Circle(&img, image.Pt(w-(d+r), h-(3*d+5*r)), r, colors[int(p.lrhc.ll.c)], -1)
	gocv.Circle(&img, image.Pt(w-(d+r), h-(2*d+3*r)), r, colors[int(p.lrhc.l.c)], -1)
	gocv.Circle(&img, image.Pt(w-(d+r), h-(d+r)), r, colors[int(p.lrhc.m.c)], -1)
	gocv.Circle(&img, image.Pt(w-(2*d+3*r), h-(d+r)), r, colors[int(p.lrhc.r.c)], -1)
	gocv.Circle(&img, image.Pt(w-(3*d+5*r), h-(d+r)), r, colors[int(p.lrhc.rr.c)], -1)

	gocv.Circle(&img, image.Pt(3*d+5*r, h-(d+r)), r, colors[int(p.llhc.ll.c)], -1)
	gocv.Circle(&img, image.Pt(2*d+3*r, h-(d+r)), r, colors[int(p.llhc.l.c)], -1)
	gocv.Circle(&img, image.Pt(d+r, h-(d+r)), r, colors[int(p.llhc.m.c)], -1)
	gocv.Circle(&img, image.Pt(d+r, h-(2*d+3*r)), r, colors[int(p.llhc.r.c)], -1)
	gocv.Circle(&img, image.Pt(d+r, h-(3*d+5*r)), r, colors[int(p.llhc.rr.c)], -1)

	gocv.IMWrite("out.png", img)
}
