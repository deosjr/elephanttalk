package main

import (
	//"fmt"
	//"image"
	//"image/color"

	"gocv.io/x/gocv"
)

func main() {
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
		pixelsPerCM:     9.587642397606789,
		displacement:    image.Pt(-3, -53),
		displayRatio:    0.715,
		referenceColors: []color.RGBA{{255, 49, 63, 0}, {61, 124, 67, 0}, {58, 86, 144, 0}, {255, 191, 37, 0}},
	}
*/

	vision(webcam, debugwindow, projection, cResults)
}
