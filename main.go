package main

import (
	//"fmt"
	"image"
	"image/color"
	"strings"

	"gocv.io/x/gocv"
)

// TODO: a proper database solution
var pageDB = map[uint64]page{}

func main() {

	cornerShorthand := func(debug string) corner {
		s := "rgby"
		return corner{
			ll: dot{c: dotColor(strings.IndexRune(s, rune(debug[0])))},
			l:  dot{c: dotColor(strings.IndexRune(s, rune(debug[1])))},
			m:  dot{c: dotColor(strings.IndexRune(s, rune(debug[2])))},
			r:  dot{c: dotColor(strings.IndexRune(s, rune(debug[3])))},
			rr: dot{c: dotColor(strings.IndexRune(s, rune(debug[4])))},
		}
	}
	//page1
	ulhc := cornerShorthand("ygybr")
	urhc := cornerShorthand("brgry")
	llhc := cornerShorthand("gbgyg")
	lrhc := cornerShorthand("bgryy")
	id := pageID(ulhc.id(), urhc.id(), llhc.id(), lrhc.id())
	pageDB[id] = page{id: id, code: `(claim this 'is-a 'window)`}

	//page2
	ulhc = cornerShorthand("yggyg")
	urhc = cornerShorthand("rgyrb")
	llhc = cornerShorthand("bybbg")
	lrhc = cornerShorthand("brgrg")
	id = pageID(ulhc.id(), urhc.id(), llhc.id(), lrhc.id())
    // TODO: when someone wishes... should be a third 'engine' page
    // that instead of claiming actually calculates illumination and wishes
	pageDB[id] = page{id: id, code: `(begin
        (when (is-a ,?page window) do (wish (,?page highlighted blue)))
        (when ,?someone wishes (,?page highlighted ,?color) do (claim ,?page 'highlighted ,?color))
    )`}

	webcam, err := gocv.VideoCaptureDevice(0)
	if err != nil {
		panic(err)
	}
	defer webcam.Close()

	debugwindow := gocv.NewWindow("debug")
	defer debugwindow.Close()
	projection := gocv.NewWindow("projector")
	defer projection.Close()

	//cResults := calibration(webcam, debugwindow, projection)
	//fmt.Println(cResults)

		cResults := calibrationResults{
			pixelsPerCM:     9.7543,
			displacement:    image.Pt(54, 24),
			displayRatio:    0.82,
			referenceColors: []color.RGBA{{255, 65, 61, 0}, {90, 130, 60, 0}, {76, 60, 88, 0}, {255, 109, 50, 0}},
		}
	vision(webcam, debugwindow, projection, cResults)
}
