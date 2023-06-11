package talk

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gocv.io/x/gocv"
)

type calibrationResults struct {
	pixelsPerCM     float64
	displacement    point
	displayRatio    float64
	referenceColors []color.RGBA
}

func calibration(webcam *gocv.VideoCapture, debugwindow, projection *gocv.Window) calibrationResults {
	//calibrationPage()

	// Step 1: set up beamer + webcam at a surface
	// Step 2: drag projector window to the beamer
	// Step 3: beamer projects midpoint -> position calibration pattern centered on midpoint
	// Step 4: recognise pattern and calculate pixel distances

	// TODO probably rename these
	// img: debug window output from camera
	// cimg: projector window
	img := gocv.NewMat()
	defer img.Close()

	cimg := gocv.NewMatWithSize(beamerHeight, beamerWidth, gocv.MatTypeCV8UC3)
	defer cimg.Close()
	red := color.RGBA{255, 0, 0, 0}
	green := color.RGBA{0, 255, 0, 0}
	blue := color.RGBA{0, 0, 255, 0}
	yellow := color.RGBA{255, 255, 0, 0}

	w, h := beamerWidth/2., beamerHeight/2.
	gocv.Line(&cimg, image.Pt(w-5, h), image.Pt(w+5, h), red, 2)
	gocv.Line(&cimg, image.Pt(w, h-5), image.Pt(w, h+5), red, 2)
	gocv.PutText(&cimg, "Place calibration pattern", image.Pt(w-100, h+50), 0, .5, color.RGBA{255, 255, 255, 0}, 2)

	var pattern []circle

	fi := frameInput{
		webcam:      webcam,
		debugWindow: debugwindow,
		projection:  projection,
		img:         img,
		cimg:        cimg,
	}

	if err := frameloop(fi, func(_ image.Image, spatialPartition map[image.Rectangle][]circle) {
		// find calibration pattern, draw around it
		for k, v := range spatialPartition {
			if !findCalibrationPattern(v) {
				continue
			}
			sortCirclesAsCorners(v)
			gocv.Rectangle(&img, k, red, 2)
			r := image.Rectangle{
				v[0].mid.add(point{-v[0].r, -v[0].r}).toIntPt(),
				v[3].mid.add(point{v[3].r, v[3].r}).toIntPt(),
			}
			gocv.Rectangle(&img, r, blue, 2)

			// TODO: draw indicators for horizontal/vertical align
			pattern = v
		}

	}, nil, 100); err != nil {
		return calibrationResults{}
	}

	// keypress breaks the loop, assume pattern is found over midpoint
	// draw conclusions about colors and distances
	webcamMid := circlesMidpoint(pattern)
	// average over all 4 distances to circles in pattern
	dpixels := euclidian(pattern[0].mid.sub(webcamMid))
	dpixels += euclidian(pattern[1].mid.sub(webcamMid))
	dpixels += euclidian(pattern[2].mid.sub(webcamMid))
	dpixels += euclidian(pattern[3].mid.sub(webcamMid))
	dpixels = dpixels / 4.
	dcm := math.Sqrt(1.5*1.5 + 1.5*1.5)

	// just like for printing 1cm = 118px, we need a new ratio for projections
	// NOTE: pixPerCM lives in webcamspace, NOT beamerspace
	pixPerCM := dpixels / dcm

	// beamer midpoint vs webcam midpoint displacement
	beamerMid := point{float64(w), float64(h)}
	displacement := beamerMid.sub(webcamMid)

	// get color samples of the four dots as reference values
	var colorSamples []color.RGBA
	for {
		if ok := webcam.Read(&img); !ok {
			fmt.Printf("cannot read device\n")
			return calibrationResults{}
		}
		if img.Empty() {
			continue
		}

		actualImage, _ := img.ToImage()
		if colorSamples == nil {
			// TODO: average within the circle?
			colorSamples = make([]color.RGBA, 4)
			for i, circle := range pattern {
				c := actualImage.At(int(circle.mid.x), int(circle.mid.y))
				rr, gg, bb, _ := c.RGBA()
				colorSamples[i] = color.RGBA{uint8(rr), uint8(gg), uint8(bb), 0}
			}
		}
		break
	}

	// project another reference point and calculate diff between webcam-space and projector-space
	// ratio between webcam and beamer
	displayRatio := 1.0

	if err := frameloop(fi, func(_ image.Image, spatialPartition map[image.Rectangle][]circle) {
		gocv.Rectangle(&cimg, image.Rect(0, 0, beamerWidth, beamerHeight), color.RGBA{}, -1)
		gocv.Line(&cimg, image.Pt(w-5+200, h), image.Pt(w+5+200, h), red, 2)
		gocv.Line(&cimg, image.Pt(w+200., h-5), image.Pt(w+200, h+5), red, 2)
		gocv.PutText(&cimg, "Place calibration pattern", image.Pt(w-100+200, h+50), 0, .5, color.RGBA{255, 255, 255, 0}, 2)

		// find calibration pattern, draw around it
		for k, v := range spatialPartition {
			if !findCalibrationPattern(v) {
				continue
			}
			sortCirclesAsCorners(v)
			gocv.Rectangle(&img, k, red, 2)
			r := image.Rectangle{
				v[0].mid.add(point{-v[0].r, -v[0].r}).toIntPt(),
				v[3].mid.add(point{v[3].r, v[3].r}).toIntPt(),
			}
			gocv.Rectangle(&img, r, blue, 2)

			midpoint := circlesMidpoint(v)
			// assume Y component stays 0 (i.e. we are horizontally aligned between webcam and beamer)
			displayRatio = midpoint.sub(webcamMid).x / 200.0

			// projecting the draw ratio difference
			withoutRatio := midpoint.add(displacement).toIntPt()
			gocv.Line(&cimg, beamerMid.toIntPt(), withoutRatio, blue, 2)

			// TODO: draw indicators for horizontal/vertical align
			pattern = v
		}

		gocv.Circle(&img, image.Pt(10, 10), 10, colorSamples[0], -1)
		gocv.Circle(&img, image.Pt(30, 10), 10, colorSamples[1], -1)
		gocv.Circle(&img, image.Pt(50, 10), 10, colorSamples[2], -1)
		gocv.Circle(&img, image.Pt(70, 10), 10, colorSamples[3], -1)

	}, colorSamples, 100); err != nil {
		return calibrationResults{}
	}

	if err := frameloop(fi, func(actualImage image.Image, spatialPartition map[image.Rectangle][]circle) {
		gocv.Rectangle(&cimg, image.Rect(0, 0, beamerWidth, beamerHeight), color.RGBA{}, -1)

		for k, v := range spatialPartition {
			if !findCalibrationPattern(v) {
				continue
			}
			sortCirclesAsCorners(v)

			colorDiff := make([]float64, 4)
			for i, circle := range v {
				c := actualImage.At(int(circle.mid.x), int(circle.mid.y))
				colorDiff[i] = colorDistance(c, colorSamples[i])
			}
			// experimentally, all diffs under 10k means we are good (paper rightway up)
			// unless ofc lighting changes drastically

			gocv.Rectangle(&img, k, red, 2)
			r := image.Rectangle{
				v[0].mid.add(point{-v[0].r, -v[0].r}).toIntPt(),
				v[3].mid.add(point{v[3].r, v[3].r}).toIntPt(),
			}
			gocv.Rectangle(&img, r, blue, 2)

			gocv.Circle(&img, v[0].mid.toIntPt(), int(v[0].r), red, 2)
			gocv.Circle(&img, v[1].mid.toIntPt(), int(v[1].r), green, 2)
			gocv.Circle(&img, v[2].mid.toIntPt(), int(v[2].r), blue, 2)
			gocv.Circle(&img, v[3].mid.toIntPt(), int(v[3].r), yellow, 2)

			// now we project around the whole A4 containing the calibration pattern
			// a4 in cm: 21 x 29.7
			a4hpx := (29.7 * pixPerCM) / 2.
			a4wpx := (21.0 * pixPerCM) / 2.
			midpoint := circlesMidpoint(v)
			min := midpoint.add(point{-a4wpx, -a4hpx})
			max := midpoint.add(point{a4wpx, a4hpx})
			a4 := image.Rectangle{min.toIntPt(), max.toIntPt()}
			gocv.Rectangle(&img, a4, blue, 4)

			// adjust for displacement and display ratio
			a4 = image.Rectangle{translate(min, displacement, displayRatio).toIntPt(), translate(max, displacement, displayRatio).toIntPt()}
			gocv.Rectangle(&cimg, a4, blue, 4)
		}

		gocv.Circle(&img, image.Pt(10, 10), 10, colorSamples[0], -1)
		gocv.Circle(&img, image.Pt(30, 10), 10, colorSamples[1], -1)
		gocv.Circle(&img, image.Pt(50, 10), 10, colorSamples[2], -1)
		gocv.Circle(&img, image.Pt(70, 10), 10, colorSamples[3], -1)
	}, colorSamples, 100); err != nil {
		return calibrationResults{}
	}

	// TODO: happy (y/n) ? if no return to start of calibration
	return calibrationResults{pixPerCM, displacement, displayRatio, colorSamples}
}
