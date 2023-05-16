package main

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gocv.io/x/gocv"
)

type circle struct {
	mid image.Point
	r   int
	c   color.Color
}

func detect(img gocv.Mat, ref []color.RGBA) map[image.Rectangle][]circle {
	cimg := gocv.NewMat()
	defer cimg.Close()

	gocv.GaussianBlur(img, &cimg, image.Pt(9, 9), 2.0, 2.0, gocv.BorderDefault)

	gocv.CvtColor(cimg, &cimg, gocv.ColorRGBToGray)

	circleMat := gocv.NewMat()
	defer circleMat.Close()

	gocv.HoughCirclesWithParams(
		cimg,
		&circleMat,
		gocv.HoughGradient,
		1,                      // dp
		float64(img.Rows()/32), // minDist
		75,                     // param1
		20,                     // param2
		1,                      // minRadius
		100,                    // maxRadius
	)

	spatialPartition := map[image.Rectangle][]circle{}
	// webcam is 1280x720, 16x9 times 80
	// TODO: more than one size, hierarchical division?
	//square := 80
	square := 120
	square2 := square / 2.
	for x := 0; x < 32; x++ {
		for y := 0; y < 18; y++ {
			ulhc := image.Pt(x*square2, y*square2)
			urhc := image.Pt(x*square2+square, y*square2+square)
			spatialPartition[image.Rectangle{ulhc, urhc}] = []circle{}
		}
	}

	actualImage, _ := img.ToImage()
	for i := 0; i < circleMat.Cols(); i++ {
		v := circleMat.GetVecfAt(0, i)
		// if circles are found
		if len(v) > 2 {
			x := int(v[0])
			y := int(v[1])
			r := int(v[2])
			c := actualImage.At(x, y)

			// if we have sampled colors, only consider circles with color 'close' to a reference
			if ref != nil {
				closeEnough := false
				for _, refC := range ref {
					if colorDistance(c, refC) < 20000 {
						closeEnough = true
					}
				}
				if !closeEnough {
					continue
				}
			}

			mid := image.Pt(x, y)
			for rect, list := range spatialPartition {
				if mid.In(rect) {
					spatialPartition[rect] = append(list, circle{mid, r, c})
				}
			}

			gocv.Circle(&img, mid, r, color.RGBA{0, 0, 255, 0}, 2)
			gocv.Circle(&img, mid, 2, color.RGBA{255, 0, 0, 0}, 3)
		}
	}
	return spatialPartition
}

func vision(webcam *gocv.VideoCapture, debugwindow, projection *gocv.Window, cResults calibrationResults) {
	w, h := 1280, 720
	img := gocv.NewMat()
	defer img.Close()
	cimg := gocv.NewMatWithSize(h, w, gocv.MatTypeCV8UC3)
	defer cimg.Close()

	for {
		if ok := webcam.Read(&img); !ok {
			fmt.Printf("cannot read device\n")
			return
		}
		if img.Empty() {
			continue
		}

		gocv.Circle(&img, image.Pt(5, 5), 5, cResults.referenceColors[0], -1)
		gocv.Circle(&img, image.Pt(15, 5), 5, cResults.referenceColors[1], -1)
		gocv.Circle(&img, image.Pt(25, 5), 5, cResults.referenceColors[2], -1)
		gocv.Circle(&img, image.Pt(35, 5), 5, cResults.referenceColors[3], -1)

		red := color.RGBA{255, 0, 0, 0}
		green := color.RGBA{0, 255, 0, 0}
		blue := color.RGBA{0, 0, 255, 0}
		yellow := color.RGBA{255, 255, 0, 0}

		gocv.Rectangle(&cimg, image.Rect(0, 0, w, h), color.RGBA{}, -1)

		spatialPartition := detect(img, cResults.referenceColors)

		// TODO: this is cheating, will work for now
		// deduplication due to overlapping detection regions
		corners := map[image.Point]struct{}{}

		// find corners
		for k, v := range spatialPartition {
			corner, ok := findCorners(v, cResults.referenceColors)
			if !ok {
				continue
			}
			gocv.Rectangle(&img, k, red, 2)
			gocv.Line(&img, corner.m.p, corner.ll.p, blue, 2)
			gocv.Line(&img, corner.m.p, corner.rr.p, blue, 2)
			//gocv.PutText(&img, corner.debugPrint(), corner.m.p.Add(image.Pt(10,20)), 0, .5, color.RGBA{}, 2)
			gocv.PutText(&img, fmt.Sprintf("%010b", corner.id()), corner.m.p.Add(image.Pt(10, 20)), 0, .5, color.RGBA{}, 2)
			cs := []color.RGBA{red, green, blue, yellow}
			gocv.Circle(&img, corner.ll.p, 8, cs[int(corner.ll.c)], -1)
			gocv.Circle(&img, corner.l.p, 8, cs[int(corner.l.c)], -1)
			gocv.Circle(&img, corner.m.p, 8, cs[int(corner.m.c)], -1)
			gocv.Circle(&img, corner.r.p, 8, cs[int(corner.r.c)], -1)
			gocv.Circle(&img, corner.rr.p, 8, cs[int(corner.rr.c)], -1)

			/*
			               rot1 := rotateAround(corner.m.p, corner.ll.p, math.Pi/2.)
			               rot2 := rotateAround(corner.m.p, corner.rr.p, math.Pi/2.)
			   			gocv.Line(&img, corner.m.p, rot1, red, 5)
			   			gocv.Line(&img, corner.m.p, rot2, red, 5)
			*/

			corners[corner.m.p] = struct{}{}
		}

		if len(corners) == 3 || len(corners) == 4 {
			pts := []image.Point{}
			for k := range corners {
				pts = append(pts, k)
			}
			r := ptsToRect(pts)
			gocv.Rectangle(&img, r, green, 2)
			r = r.Inset(int(3 * cResults.pixelsPerCM))
			r = image.Rectangle{translate(r.Min, cResults.displacement, cResults.displayRatio), translate(r.Max, cResults.displacement, cResults.displayRatio)}
			gocv.Rectangle(&cimg, r, green, -1)
			t := r.Min.Add(image.Pt(r.Dx()/4., r.Dy()/2.))
			gocv.PutText(&cimg, "Hello World!", t, 0, .5, red, 2)
		}

		debugwindow.IMShow(img)
		projection.IMShow(cimg)
		if debugwindow.WaitKey(10) >= 0 {
			break
		}
	}
}

func euclidian(p image.Point) float64 {
	return math.Sqrt(float64(p.X*p.X + p.Y*p.Y))
}

// clockwise rotation
// TODO: ???? expected counterclockwise ????
func rotateAround(pivot, point image.Point, radians float64) image.Point {
	s := math.Sin(radians)
	c := math.Cos(radians)

	x := float64(point.X - pivot.X)
	y := float64(point.Y - pivot.Y)

	xNew := (c*x - s*y) + float64(pivot.X)
	yNew := (s*x + c*y) + float64(pivot.Y)
	return image.Pt(int(xNew), int(yNew))
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
