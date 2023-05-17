package main

import (
	//"fmt"
	//"image"
	//"image/color"
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
	ulhc := cornerShorthand("bbrrg")
	urhc := cornerShorthand("rybbg")
	llhc := cornerShorthand("brbyy")
	lrhc := cornerShorthand("rybgg")
	id := pageID(ulhc.id(), urhc.id(), llhc.id(), lrhc.id())
	pageDB[id] = page{id: id, code: "Hello"}

	//page2
	ulhc = cornerShorthand("bgrrb")
	urhc = cornerShorthand("rybgg")
	llhc = cornerShorthand("rbryg")
	lrhc = cornerShorthand("brgrb")
	id = pageID(ulhc.id(), urhc.id(), llhc.id(), lrhc.id())
	pageDB[id] = page{id: id, code: "World!"}

	webcam, err := gocv.VideoCaptureDevice(0)
	if err != nil {
		panic(err)
	}
	defer webcam.Close()

	debugwindow := gocv.NewWindow("debug")
	defer debugwindow.Close()
	projection := gocv.NewWindow("projector")
	defer projection.Close()

	cResults := calibration(webcam, debugwindow, projection)
	//fmt.Println(cResults)

	/*
		cResults := calibrationResults{
			pixelsPerCM:     9.502872709515637,
			displacement:    image.Pt(6, -28),
			displayRatio:    0.725,
			referenceColors: []color.RGBA{{216, 74, 67, 0}, {74, 123, 76, 0}, {93, 96, 136, 0}, {241, 190, 89, 0}},
		}
	*/

	vision(webcam, debugwindow, projection, cResults)
}
