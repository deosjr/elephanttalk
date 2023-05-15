package main

import (
    "fmt"
	"image"
	"image/color"
    "math"
    "sort"

	"gocv.io/x/gocv"
)

type calibrationResults struct {
    pixelsPerCM float64
    displacement image.Point
    displayRatio float64
    referenceColors []color.RGBA
}

func calibration(webcam *gocv.VideoCapture, debugwindow, projection *gocv.Window) calibrationResults {
    //calibrationPage()
    
    // Step 1: set up beamer + webcam at a surface
    // Step 2: drag projector window to the beamer
    // Step 3: beamer projects midpoint -> position calibration pattern centered on midpoint
    // Step 4: recognise pattern and calculate pixel distances

	img := gocv.NewMat()
	defer img.Close()

    w, h := 1280, 720
	cimg := gocv.NewMatWithSize(h, w, gocv.MatTypeCV8UC3)
	defer cimg.Close()
	red := color.RGBA{255, 0, 0, 0}
	green := color.RGBA{0, 255, 0, 0}
	blue := color.RGBA{0, 0, 255, 0}
	yellow := color.RGBA{255, 255, 0, 0}

    gocv.Line(&cimg, image.Pt(w/2.-5, h/2.), image.Pt(w/2.+5, h/2.), red, 2)
    gocv.Line(&cimg, image.Pt(w/2., h/2.-5), image.Pt(w/2., h/2.+5), red, 2)
    gocv.PutText(&cimg, "Place calibration pattern", image.Pt(w/2.-100, h/2.+50), 0, .5, color.RGBA{255,255,255,0}, 2)

    var pattern []circle

    for {
		if ok := webcam.Read(&img); !ok {
			fmt.Printf("cannot read device\n")
			return calibrationResults{}
		}
		if img.Empty() {
			continue
		}

        spatialPartition := detect(img, nil)

        // find calibration pattern, draw around it
        for k, v := range spatialPartition {
            if !findCalibrationPattern(v) {
                continue
            }
            // ulhc, urhc, llhc, lrhc
            sort.Slice(v, func(i, j int) bool {
                return v[i].mid.X + v[i].mid.Y < v[j].mid.X + v[j].mid.Y
            })
            if v[1].mid.Y > v[2].mid.Y {
                v[1], v[2] = v[2], v[1]
            }
            gocv.Rectangle(&img, k, red, 2)
            r := image.Rectangle{v[0].mid.Add(image.Pt(-v[0].r, -v[0].r)), v[3].mid.Add(image.Pt(v[3].r, v[3].r))}
            gocv.Rectangle(&img, r, blue, 2)

            // TODO: draw indicators for horizontal/vertical align
            pattern = v
        }

        debugwindow.IMShow(img)
        projection.IMShow(cimg)
        key := debugwindow.WaitKey(100)
		if key >= 0 {
            fmt.Println(key)
			break
		}
    }

    // keypress breaks the loop, assume pattern is found over midpoint
    // draw conclusions about colors and distances
    webcamMid := pattern[0].mid
    for _, p := range pattern[1:] {
        webcamMid = webcamMid.Add(p.mid)
    }
    webcamMid = webcamMid.Div(len(pattern))
    // average over all 4 distances to circles in pattern
    dpixels := euclidian(pattern[0].mid.Sub(webcamMid))
    dpixels += euclidian(pattern[1].mid.Sub(webcamMid))
    dpixels += euclidian(pattern[2].mid.Sub(webcamMid))
    dpixels += euclidian(pattern[3].mid.Sub(webcamMid))
    dpixels = dpixels/4.
    dcm := math.Sqrt(1.5*1.5 + 1.5*1.5)

    // just like for printing 1cm = 118px, we need a new ratio for projections
    // NOTE: pixPerCM lives in webcamspace, NOT beamerspace
    pixPerCM := dpixels / dcm

    // beamer midpoint vs webcam midpoint displacement
    beamerMid := image.Pt(w/2., h/2.)
    displacement := beamerMid.Sub(webcamMid)

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
                c := actualImage.At(circle.mid.X, circle.mid.Y)
                rr, gg, bb, _ := c.RGBA()
                colorSamples[i] = color.RGBA{uint8(rr), uint8(gg), uint8(bb), 0}
            }
        }
        break
    }
    
    // project another reference point and calculate diff between webcam-space and projector-space
    // ratio between webcam and beamer
    displayRatio := 1.0

    for {
		if ok := webcam.Read(&img); !ok {
			fmt.Printf("cannot read device\n")
			return calibrationResults{}
		}
		if img.Empty() {
			continue
		}

        gocv.Rectangle(&cimg, image.Rect(0, 0, w, h), color.RGBA{}, -1)
        gocv.Line(&cimg, image.Pt(w/2.-5+200, h/2.), image.Pt(w/2.+5+200, h/2.), red, 2)
        gocv.Line(&cimg, image.Pt(w/2+200., h/2.-5), image.Pt(w/2.+200, h/2.+5), red, 2)
        gocv.PutText(&cimg, "Place calibration pattern", image.Pt(w/2.-100+200, h/2.+50), 0, .5, color.RGBA{255,255,255,0}, 2)

        spatialPartition := detect(img, colorSamples)

        // find calibration pattern, draw around it
        for k, v := range spatialPartition {
            if !findCalibrationPattern(v) {
                continue
            }
            // ulhc, urhc, llhc, lrhc
            sort.Slice(v, func(i, j int) bool {
                return v[i].mid.X + v[i].mid.Y < v[j].mid.X + v[j].mid.Y
            })
            if v[1].mid.Y > v[2].mid.Y {
                v[1], v[2] = v[2], v[1]
            }
            gocv.Rectangle(&img, k, red, 2)
            r := image.Rectangle{v[0].mid.Add(image.Pt(-v[0].r, -v[0].r)), v[3].mid.Add(image.Pt(v[3].r, v[3].r))}
            gocv.Rectangle(&img, r, blue, 2)

            midpoint := v[0].mid
            for _, p := range v[1:] {
                midpoint = midpoint.Add(p.mid)
            }
            midpoint = midpoint.Div(len(v))
            // assume Y component stays 0 (i.e. we are horizontally aligned between webcam and beamer)
            displayRatio = float64(midpoint.Sub(webcamMid).X) / 200.0

            // projecting the draw ratio difference
            withoutRatio := midpoint.Add(displacement)
            gocv.Line(&cimg, beamerMid, withoutRatio, blue, 2)

            // TODO: draw indicators for horizontal/vertical align
            pattern = v
        }

    	gocv.Circle(&img, image.Pt(10,10), 10, colorSamples[0], -1)
    	gocv.Circle(&img, image.Pt(30,10), 10, colorSamples[1], -1)
    	gocv.Circle(&img, image.Pt(50,10), 10, colorSamples[2], -1)
    	gocv.Circle(&img, image.Pt(70,10), 10, colorSamples[3], -1)

        debugwindow.IMShow(img)
        projection.IMShow(cimg)
        key := debugwindow.WaitKey(100)
		if key >= 0 {
            fmt.Println(key)
			break
		}
    }

    for {
		if ok := webcam.Read(&img); !ok {
			fmt.Printf("cannot read device\n")
			return calibrationResults{}
		}
		if img.Empty() {
			continue
		}

        gocv.Rectangle(&cimg, image.Rect(0, 0, w, h), color.RGBA{}, -1)

        actualImage, _ := img.ToImage()
        spatialPartition := detect(img, colorSamples)
        for k, v := range spatialPartition {
            if !findCalibrationPattern(v) {
                continue
            }
            // ulhc, urhc, llhc, lrhc
            sort.Slice(v, func(i, j int) bool {
                return v[i].mid.X + v[i].mid.Y < v[j].mid.X + v[j].mid.Y
            })
            if v[1].mid.Y > v[2].mid.Y {
                v[1], v[2] = v[2], v[1]
            }

            colorDiff := make([]float64, 4)
            for i, circle := range v {
                c := actualImage.At(circle.mid.X, circle.mid.Y)
                colorDiff[i] = colorDistance(c, colorSamples[i])
            }
            // experimentally, all diffs under 10k means we are good (paper rightway up)
            // unless ofc lighting changes drastically

            gocv.Rectangle(&img, k, red, 2)
            minv0r := -v[0].r
            v3r := v[3].r
            r := image.Rectangle{v[0].mid.Add(image.Pt(minv0r, minv0r)), v[3].mid.Add(image.Pt(v3r, v3r))}
            gocv.Rectangle(&img, r, blue, 2)

            gocv.Circle(&img, v[0].mid, v[0].r, red, 2)
            gocv.Circle(&img, v[1].mid, v[1].r, green, 2)
            gocv.Circle(&img, v[2].mid, v[2].r, blue, 2)
            gocv.Circle(&img, v[3].mid, v[3].r, yellow, 2)

            // now we project around the whole A4 containing the calibration pattern
            // a4 in cm: 21 x 29.7
            a4hpx := int((29.7*pixPerCM)/2.)
            a4wpx := int((21.0*pixPerCM)/2.)
            midpoint := v[0].mid
            for _, p := range v[1:] {
                midpoint = midpoint.Add(p.mid)
            }
            midpoint = midpoint.Div(len(v))
            a4 := image.Rectangle{midpoint.Add(image.Pt(-a4wpx,-a4hpx)), midpoint.Add(image.Pt(a4wpx,a4hpx))}
            gocv.Rectangle(&img, a4, blue, 4)

            // adjust for displacement and display ratio
            a4 = image.Rectangle{translate(a4.Min, displacement, displayRatio), translate(a4.Max, displacement, displayRatio)}
            gocv.Rectangle(&cimg, a4, blue, 4)
        }

    	gocv.Circle(&img, image.Pt(10,10), 10, colorSamples[0], -1)
    	gocv.Circle(&img, image.Pt(30,10), 10, colorSamples[1], -1)
    	gocv.Circle(&img, image.Pt(50,10), 10, colorSamples[2], -1)
    	gocv.Circle(&img, image.Pt(70,10), 10, colorSamples[3], -1)

        debugwindow.IMShow(img)
        projection.IMShow(cimg)
        key := debugwindow.WaitKey(100)
		if key >= 0 {
            fmt.Println(key)
			break
		}
    }
    // TODO: happy (y/n) ? if no return to start of calibration
    return calibrationResults{pixPerCM, displacement, displayRatio, colorSamples}
}

// calibration pattern is four circles in a rectangle
// check if they are equidistant to their midpoint
func findCalibrationPattern(v []circle) bool {
    if len(v) != 4 {
        return false
    }
    midpoint := v[0].mid
    for _, p := range v[1:] {
        midpoint = midpoint.Add(p.mid)
    }
    midpoint = midpoint.Div(len(v))

    dist := euclidian(midpoint.Sub(v[0].mid))
    for _, p := range v[1:] {
        ddist := euclidian(midpoint.Sub(p.mid))
        if !equalWithMargin(ddist, dist, 2.0) {
            return false
        }
    }
    return true
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

func translate(p, delta image.Point, ratio float64) image.Point {
    // first we add the difference from webcam to beamer midpoints
    q := p.Add(delta) 
    // then we boost from midpoint by missing ratio
    beamerMid := image.Pt(1280/2., 720/2.)
    deltaV := q.Sub(beamerMid)
    adjust := image.Pt(int(float64(deltaV.X) * ((1./ratio) - 1)), int(float64(deltaV.Y) * ((1./ratio) - 1)))
    return q.Add(adjust)
}
