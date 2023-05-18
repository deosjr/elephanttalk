package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"time"

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
		5,                      // minRadius
		50,                     // maxRadius
	)

	spatialPartition := map[image.Rectangle][]circle{}
	// webcam is 1280x720, 16x9 times 80
	// TODO: more than one size, hierarchical division?
	//square := 80
	square := 130
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
			// TODO: we could use gocv.InRange using NewMatFromScalar for lower/upper bounds then bitwiseOr img per color
			// then join back(?) the four color-filtered versions of the image and only test Hough against that?
			if ref != nil {
				closeEnough := false
				for _, refC := range ref {
					if colorDistance(c, refC) < 30000 {
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
		start := time.Now()
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
		cornersByTop := map[image.Point]corner{}

		// find corners
		for k, v := range spatialPartition {
			corner, ok := findCorners(v, cResults.referenceColors)
			if !ok {
				continue
			}
			gocv.Rectangle(&img, k, red, 2)
			gocv.Line(&img, corner.m.p, corner.ll.p, blue, 2)
			gocv.Line(&img, corner.m.p, corner.rr.p, blue, 2)
			//gocv.PutText(&img, fmt.Sprintf("%010b", corner.id()), corner.m.p.Add(image.Pt(10, 40)), 0, .5, color.RGBA{}, 2)

			// calculate angle between right arm of corner and absolute right in webcam space
			rightArm := corner.rr.p.Sub(corner.m.p)
			rightAbs := corner.m.p.Add(image.Pt(100, 0)).Sub(corner.m.p)
			dot := float64(rightArm.X*rightAbs.X + rightArm.Y*rightAbs.Y)
			angle := math.Acos(dot / (euclidian(rightArm) * euclidian(rightAbs)))
			if corner.rr.p.Y < corner.m.p.Y {
				angle = 2*math.Pi - angle
			}
			gocv.PutText(&img, fmt.Sprintf("%f", angle), corner.m.p.Add(image.Pt(10, 20)), 0, .5, color.RGBA{}, 2)

			cs := []color.RGBA{red, green, blue, yellow}
			gocv.Circle(&img, corner.ll.p, 8, cs[int(corner.ll.c)], -1)
			gocv.Circle(&img, corner.l.p, 8, cs[int(corner.l.c)], -1)
			gocv.Circle(&img, corner.m.p, 8, cs[int(corner.m.c)], -1)
			gocv.Circle(&img, corner.r.p, 8, cs[int(corner.r.c)], -1)
			gocv.Circle(&img, corner.rr.p, 8, cs[int(corner.rr.c)], -1)

			cornersByTop[corner.m.p] = corner
		}

		corners := []corner{}
		for _, c := range cornersByTop {
			corners = append(corners, c)
		}
		cornerMap := map[corner]corner{}
		// compare each corner against all others (TODO: can be more efficient ofc)
		// try to find another corner: the one clockwise in order that would form a page
		for _, c := range corners {
			for _, o := range corners {
				if c.m.p == o.m.p {
					continue
				}
				right := c.rr.p.Sub(c.m.p)
				toO := o.m.p.Sub(c.m.p)
				dot1 := float64(right.X*toO.X + right.Y*toO.Y)
				angle1 := math.Acos(dot1 / (euclidian(right) * euclidian(toO)))
				left := o.ll.p.Sub(o.m.p)
				toC := c.m.p.Sub(o.m.p)
				dot2 := float64(left.X*toC.X + left.Y*toC.Y)
				angle2 := math.Acos(dot2 / (euclidian(left) * euclidian(toC)))
				if angle1 > 0.05 || angle2 > 0.05 {
					continue
				}
				prev, ok := cornerMap[c]
				if ok {
					// overwrite previously found corner if this one is closer
					if euclidian(c.m.p.Sub(prev.m.p)) > euclidian(c.m.p.Sub(o.m.p)) {
						cornerMap[c] = o
					}
				} else {
					cornerMap[c] = o
				}
				break
			}
		}

		// parse corners into pages
		pages := []page{}
		for len(cornerMap) > 0 {
			// pick a random starting corner from the map
			var c, next corner
			for k, v := range cornerMap {
				c, next = k, v
				break
			}
			delete(cornerMap, c)
			cs := []corner{c, next}
			// TODO: only picking perfect info pages atm, ie. those with 4 corners recognized
			for i := 0; i < 3; i++ {
				n, ok := cornerMap[next]
				if !ok {
					break
				}
				delete(cornerMap, next)
				cs = append(cs, n)
				c, next = next, n
			}
			if len(cs) != 5 || cs[0].m.p != cs[4].m.p {
				continue
			}
			cs = cs[:4]
			// assumption: ulhc is indeed upper left hand corner! (TODO: rotation!)
			sort.Slice(cs, func(i, j int) bool {
				return cs[i].m.p.X+cs[i].m.p.Y < cs[j].m.p.X+cs[j].m.p.Y
			})
			if cs[1].m.p.Y > cs[2].m.p.Y {
				cs[1], cs[2] = cs[2], cs[1]
			}
			p := page{ulhc: cs[0], urhc: cs[1], llhc: cs[2], lrhc: cs[3]}
			pID := pageID(p.ulhc.id(), p.urhc.id(), p.lrhc.id(), p.llhc.id())
			p.id = pID
			pp, ok := pageDB[pID]
			if ok {
				p.code = pp.code
			}
			pages = append(pages, p)
		}

		for _, page := range pages {
			pts := []image.Point{page.ulhc.m.p, page.urhc.m.p, page.llhc.m.p, page.lrhc.m.p}
			center := pts[0].Add(pts[1]).Add(pts[2]).Add(pts[3]).Div(4)
			rightArm := page.ulhc.rr.p.Sub(page.ulhc.m.p)
			rightAbs := page.ulhc.m.p.Add(image.Pt(100, 0)).Sub(page.ulhc.m.p)
			dot := float64(rightArm.X*rightAbs.X + rightArm.Y*rightAbs.Y)
			angle := math.Acos(dot / (euclidian(rightArm) * euclidian(rightAbs)))
			if page.ulhc.rr.p.Y < page.ulhc.m.p.Y {
				angle = 2*math.Pi - angle
			}
			r := ptsToRect([]image.Point{
				rotateAround(center, pts[0], angle),
				rotateAround(center, pts[1], angle),
				rotateAround(center, pts[2], angle),
				rotateAround(center, pts[3], angle),
			})
			gocv.Rectangle(&img, r, green, 2)

			aabb := ptsToRect(pts)
			gocv.Rectangle(&img, aabb, blue, 2)

			//TODO: all in one scale/rotate/translate
			// see https://github.com/milosgajdos/gocv-playground/blob/master/04_Geometric_Transformations/README.md
			illu := gocv.NewMatWithSize(h, w, gocv.MatTypeCV8UC3)
			defer illu.Close()

			r = r.Inset(int(3 * cResults.pixelsPerCM))
			r = image.Rectangle{translate(r.Min, cResults.displacement, cResults.displayRatio), translate(r.Max, cResults.displacement, cResults.displayRatio)}
			gocv.Rectangle(&illu, r, green, -1)
			t := r.Min.Add(image.Pt(r.Dx()/4., r.Dy()/2.))
			text := fmt.Sprintf("NOT FOUND:\n%d", page.id)
			if page.code != "" {
				text = page.code
			}
			gocv.PutText(&illu, text, t, 0, .5, red, 2)

			center = r.Min.Add(r.Max).Div(2)
			angle = -(angle / (2 * math.Pi)) * 360.
			m := gocv.GetRotationMatrix2D(center, angle, 1.0)
			cillu := gocv.NewMat()
			defer cillu.Close()
			gocv.WarpAffine(illu, &cillu, m, image.Pt(w, h))

			blit(&cillu, &cimg)
		}

		fps := time.Second / time.Since(start)
		gocv.PutText(&img, fmt.Sprintf("FPS: %d", fps), image.Pt(0, 20), 0, .5, color.RGBA{}, 2)

		debugwindow.IMShow(img)
		projection.IMShow(cimg)
		if debugwindow.WaitKey(10) >= 0 {
			break
		}
	}
}

// TODO: only works if area to be colored is still black
// smth like 'set nonblack area in 'from' to white, use that as mask, blacken 'to' area with mask first?'
func blit(from, to *gocv.Mat) {
	gocv.BitwiseOr(*from, *to, to)
}

func euclidian(p image.Point) float64 {
	return math.Sqrt(float64(p.X*p.X + p.Y*p.Y))
}

// counterclockwise rotation
// TODO: ???? expected counterclockwise but getting clockwise ????
// 'fixed' by flipping sign on angle in sin/cos, shouldnt be there
func rotateAround(pivot, point image.Point, radians float64) image.Point {
	s := math.Sin(-radians)
	c := math.Cos(-radians)

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
