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

			// if we have sampled colors, only consider circles with color 'close' to a reference
			if ref != nil {
				c := actualImage.At(x, y)
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
					spatialPartition[rect] = append(list, circle{mid, r})
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

		gocv.Rectangle(&cimg, image.Rect(0, 0, w, h), color.RGBA{}, -1)

		spatialPartition := detect(img, cResults.referenceColors)

		// this is cheating, will work for now
		corners := map[circle]struct{}{}

		// find corners
		for k, v := range spatialPartition {
			dists, ok := findCorners(v)
			if !ok {
				continue
			}
			gocv.Rectangle(&img, k, red, 2)
			gocv.Line(&img, dists[2].mid, dists[3].mid, blue, 2)
			gocv.Line(&img, dists[2].mid, dists[4].mid, blue, 2)
			corners[dists[2]] = struct{}{}
		}

		if len(corners) == 3 || len(corners) == 4 {
			cs := []circle{}
			for k := range corners {
				cs = append(cs, k)
			}
			r := circleBound(cs[0])
			for _, c := range cs[1:] {
				r = r.Union(circleBound(c))
			}
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

func circleBound(c circle) image.Rectangle {
	return image.Rectangle{
		c.mid.Add(image.Pt(-c.r, -c.r)),
		c.mid.Add(image.Pt(c.r, c.r)),
	}
}
