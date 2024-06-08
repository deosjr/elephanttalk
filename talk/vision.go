package talk

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"time"

	"github.com/deosjr/elephanttalk/opencv"
	"gocv.io/x/gocv"
)

var (
	// detected from webcam output instead!
	//webcamWidth, webcamHeight = 1280, 720
	beamerWidth, beamerHeight = 1280, 720
	// beamerWidth, beamerHeight = 1920, 1080
)

func Run() {
	webcam, err := gocv.VideoCaptureDevice(0)
	if err != nil {
		panic(err)
	}
	defer webcam.Close()

	debugwindow := gocv.NewWindow("debug")
	defer debugwindow.Close()
	projection := gocv.NewWindow("projector")
	defer projection.Close()
	// cbWindow := gocv.NewWindow("projection")
	// defer cbWindow.Close()

	filename := "calibration.json"
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		fmt.Println("Calibration file does not exist, starting calibration")
		scChsBrd := chessBoardCalibration(webcam, debugwindow, projection)
		if len(scChsBrd.ColorModels) > 0 {
			defer scChsBrd.Close()
			detectPattern(webcam, debugwindow, projection, scChsBrd)
		}

	} else {
		fmt.Println("Calibration file exists, skipping calibration")
		scChsBrd := loadCalibration(filename)
		vision(webcam, debugwindow, projection, scChsBrd)
		// if len(scChsBrd.ColorModels) > 0 {
		// 	defer scChsBrd.Close()
		// 	// detectPattern(webcam, debugwindow, projection, scChsBrd)
		// }
	}
}

type frameInput struct {
	webcam      *gocv.VideoCapture
	debugWindow *gocv.Window
	projection  *gocv.Window
	// TODO: should these be passed as ptrs?
	img  gocv.Mat
	cimg gocv.Mat

	calibBoard straightChessboard
}

func colorRGBAtoScalar(c color.RGBA) gocv.Scalar {
	return gocv.NewScalar(float64(c.R), float64(c.G), float64(c.B), 0)
}

func frameloop(fi frameInput, f func(image.Image, map[image.Rectangle][]circle, []color.RGBA), waitMillis int) error {
	// isFirstIter := true
	// K := NB_CLRD_CHCKRS + 1
	labels := gocv.NewMat()
	defer labels.Close()
	centers := gocv.NewMat()
	defer centers.Close()
	// termCriteria := gocv.NewTermCriteria(gocv.EPS+gocv.Count, 10, 1.0)

	cbCanvas := gocv.NewMatWithSize(chessHeight*blockHeight, chessWidth*blockWidth, gocv.MatTypeCV8UC3)
	defer cbCanvas.Close()

	for {
		start := time.Now()
		if ok := fi.webcam.Read(&fi.img); !ok {
			return fmt.Errorf("cannot read device\n")
		}
		if fi.img.Empty() {
			continue
		}

		frameRegion := beamerToChessboard(fi.img, fi.calibBoard)
		defer frameRegion.Close()

		// kMeansImage := kMeansFrame(frameRegion, K, &labels, &centers, termCriteria, isFirstIter)
		// defer kMeansImage.Close()

		// fmt.Println("centers")
		// PrintMatValues32F(centers)
		centers_32F, _ := centers.DataPtrFloat32()
		referenceColorsUO := []color.RGBA{}
		for i := 0; i < centers.Rows(); i++ {
			// OPENCV IS BGR!
			referenceColorsUO = append(referenceColorsUO, color.RGBA{uint8(centers_32F[i*3+2]), uint8(centers_32F[i*3+1]), uint8(centers_32F[i*3]), 0})
		}

		referenceColors := []color.RGBA{}
		for c := 0; c < len(cornerColors); c++ {
			bestColor := color.RGBA{}
			bestColorDistance := 1000000.0
			for r := 0; r < len(referenceColorsUO); r++ {
				statColor := colorRGBAtoScalar(cornerColors[c])
				refColor := colorRGBAtoScalar(referenceColorsUO[r])
				statColorHSV := gocv.NewMat()
				defer statColorHSV.Close()
				refColorHSV := gocv.NewMat()
				defer refColorHSV.Close()
				gocv.CvtColor(gocv.NewMatFromScalar(statColor, gocv.MatTypeCV8UC3), &statColorHSV, gocv.ColorBGRToHSV)
				gocv.CvtColor(gocv.NewMatFromScalar(refColor, gocv.MatTypeCV8UC3), &refColorHSV, gocv.ColorBGRToHSV)

				colorDistance := math.Abs(float64(statColorHSV.GetUCharAt3(0, 0, 0)) - float64(refColorHSV.GetUCharAt3(0, 0, 0)))
				if colorDistance < bestColorDistance {
					bestColorDistance = colorDistance
					bestColor = referenceColorsUO[r]
				}
			}

			referenceColors = append(referenceColors, bestColor)
		}
		// isFirstIter = false

		// since detect draws in img, we take a snapshot first
		actualImage, _ := fi.img.ToImage()
		spatialPartition := detect(frameRegion, actualImage, referenceColors)

		f(actualImage, spatialPartition, referenceColors)

		fps := time.Second / time.Since(start)
		gocv.PutText(&fi.img, fmt.Sprintf("FPS: %d", fps), image.Pt(0, 20), 0, .5, color.RGBA{}, 2)

		fi.debugWindow.IMShow(fi.img)
		gocv.NewWindow("frameRegion").IMShow(frameRegion)
		key := fi.debugWindow.WaitKey(waitMillis)

		getSheet(key, &cbCanvas)

		if key >= 0 {
			return nil
		}
	}
}

func vision(webcam *gocv.VideoCapture, debugwindow, projection *gocv.Window, scChsBrd straightChessboard) {
	img := gocv.NewMat()
	defer img.Close()
	cimg := gocv.NewMatWithSize(beamerHeight, beamerWidth, gocv.MatTypeCV8UC3)
	defer cimg.Close()

	l := LoadRealTalk()
	// translate to beamerspace
	// pixPerCM := cResults.pixelsPerCM
	pixPerCM := 8.0
	// if cResults.displayRatio != 0 {
	// 	pixPerCM *= (1. / cResults.displayRatio) - 1.
	// }
	l.Eval(fmt.Sprintf("(define pixelsPerCM %f)", pixPerCM))

	fi := frameInput{
		webcam:      webcam,
		debugWindow: debugwindow,
		projection:  projection,
		img:         img,
		cimg:        cimg,
		calibBoard:  scChsBrd,
	}

	// ttl in frames; essentially buffering page location for flaky detection
	type persistPage struct {
		id  uint64
		ttl int
	}

	persistCorners := map[corner]persistPage{}

	if err := frameloop(fi, func(_ image.Image, spatialPartition map[image.Rectangle][]circle, referenceColors []color.RGBA) {

		clear(l)
		datalogIDs := map[uint64]int{}

		for k, v := range persistCorners {
			if v.ttl == 0 {
				delete(persistCorners, k)
				continue
			}
			persistCorners[k] = persistPage{v.id, v.ttl - 1}
		}

		gocv.Circle(&img, image.Pt(5, 5), 5, referenceColors[0], -1)
		gocv.Circle(&img, image.Pt(15, 5), 5, referenceColors[1], -1)
		gocv.Circle(&img, image.Pt(25, 5), 5, referenceColors[2], -1)
		gocv.Circle(&img, image.Pt(35, 5), 5, referenceColors[3], -1)
		gocv.Circle(&img, image.Pt(45, 5), 5, referenceColors[4], -1)

		referenceColors = referenceColors[1:]

		red := color.RGBA{255, 0, 0, 0}
		green := color.RGBA{0, 255, 0, 0}
		blue := color.RGBA{0, 0, 255, 0}
		yellow := color.RGBA{255, 255, 0, 0}

		gocv.Rectangle(&cimg, image.Rect(0, 0, beamerWidth, beamerHeight), color.RGBA{}, -1)

		// TODO: this is cheating, will work for now
		// deduplication due to overlapping detection regions
		cornersByTop := map[point]corner{}
		corners := []corner{}

		// find corners
		for k, v := range spatialPartition {
			corner, ok := findCorners(v, referenceColors)
			if !ok {
				continue
			}
			if _, ok := cornersByTop[corner.m.p]; ok {
				// dont care if this corner is detected in multiple overlapping regions
				continue
			}

			gocv.Rectangle(&img, k, red, 2)
			gocv.Line(&img, corner.m.p.toIntPt(), corner.ll.p.toIntPt(), blue, 2)
			gocv.Line(&img, corner.m.p.toIntPt(), corner.rr.p.toIntPt(), blue, 2)

			cs := []color.RGBA{red, green, blue, yellow}
			gocv.Circle(&img, corner.ll.p.toIntPt(), 8, cs[int(corner.ll.c)], -1)
			gocv.Circle(&img, corner.l.p.toIntPt(), 8, cs[int(corner.l.c)], -1)
			gocv.Circle(&img, corner.m.p.toIntPt(), 8, cs[int(corner.m.c)], -1)
			gocv.Circle(&img, corner.r.p.toIntPt(), 8, cs[int(corner.r.c)], -1)
			gocv.Circle(&img, corner.rr.p.toIntPt(), 8, cs[int(corner.rr.c)], -1)

			cornersByTop[corner.m.p] = corner
			corners = append(corners, corner)
		}
		// fmt.Print("corners: ", corners)

		// attempt to update corners if their colors dont match corner that was really close to it previous frame
		// persisted corners are guaranteed to have matched an existing page
		for i, c := range corners {
			for o := range persistCorners {
				if euclidian(c.m.p.sub(o.m.p)) < 5.0 {
					corners[i] = corner{
						ll: dot{c.ll.p, o.ll.c},
						l:  dot{c.l.p, o.l.c},
						m:  dot{c.m.p, o.m.c},
						r:  dot{c.r.p, o.r.c},
						rr: dot{c.rr.p, o.rr.c},
					}
					break
				}
			}
		}

		cornersClockwise := map[corner]corner{}
		cornersCounterClockwise := map[corner]corner{}
		// compare each corner against all others (TODO: can be more efficient ofc)
		// try to find another corner: the one clockwise in order that would form a page
		for _, c := range corners {
			for _, o := range corners {
				if c.m.p == o.m.p {
					continue
				}
				right := c.rr.p.sub(c.m.p)
				toO := o.m.p.sub(c.m.p)
				angle1 := angleBetween(right, toO)
				if angle1 > 0.05 {
					continue
				}
				left := o.ll.p.sub(o.m.p)
				toC := c.m.p.sub(o.m.p)
				angle2 := angleBetween(left, toC)
				if angle2 > 0.05 {
					continue
				}
				prev, ok := cornersClockwise[c]
				if ok {
					// overwrite previously found corner if this one is closer
					if euclidian(c.m.p.sub(prev.m.p)) > euclidian(c.m.p.sub(o.m.p)) {
						cornersClockwise[c] = o
						cornersCounterClockwise[o] = c
					}
				} else {
					cornersClockwise[c] = o
					cornersCounterClockwise[o] = c
				}
			}
		}

		// parse corners into pages
		pages := map[uint64]page{}
		for len(corners) > 0 {
			c := corners[0]
			next := cornersClockwise[c]
			corners = corners[1:]

			cs := []corner{c, next}
			// only picking potential pages, those with at least 3 corners recognised
			for i := 0; i < 3; i++ {
				n, ok := cornersClockwise[next]
				if !ok {
					break
				}
				cs = append(cs, n)
				c, next = next, n
			}
			if !(len(cs) == 3) && !(len(cs) == 5 && cs[0].m.p == cs[4].m.p) {
				// either we have 3 corners, or we have 5 since the last one is guaranteed to point at the first
				continue
			}
			// because cs[0] = cs[4], remove one instance of that corner
			if len(cs) == 5 {
				cs = cs[:4]
			}

			// if we detect four corners but one is wrong, we should attempt getting page from other configurations
			// if we detect only three, we attempt to find by those 3 corners only
			var p page
			if len(cs) == 3 {
				pID := pagePartialID(cs[0].id(), cs[1].id(), cs[2].id())
				pg, ok := pageDB[pID]
				if !ok {
					continue
				}
				if _, ok := pages[p.id]; ok {
					continue
				}
				p = pg
				missingMid := cs[2].m.p.add(cs[0].m.p.sub(cs[1].m.p))
				// TODO: fill in missing dots positions on missing corner?
				missingCorner := corner{m: dot{p: missingMid}}
				cs = append(cs, missingCorner)
			} else if len(cs) == 4 {
				found := false
				for i := 0; i < 4; i++ {
					cs = []corner{cs[1], cs[2], cs[3], cs[0]}
					pID := pagePartialID(cs[0].id(), cs[1].id(), cs[2].id())
					pg, ok := pageDB[pID]
					if !ok {
						continue
					}
					if _, ok := pages[p.id]; ok {
						continue
					}
					p = pg
					found = true
					break
				}
				if !found {
					continue
				}
			}
			// TODO: if ulhc is not properly detected, this will cause issues
			for i := 0; i < 4; i++ {
				if cs[0].id() == p.ulhc.id() {
					break
				}
				cs = []corner{cs[1], cs[2], cs[3], cs[0]}
			}
			// error correct colors on corners because 1 might be wrong
			// in which case we would persist wrong corner across frames and error correction will not work
			//p.ulhc, p.urhc, p.lrhc, p.llhc = cs[0], cs[1], cs[2], cs[3]
			p.ulhc = corner{
				ll: dot{cs[0].ll.p, p.ulhc.ll.c},
				l:  dot{cs[0].l.p, p.ulhc.l.c},
				m:  dot{cs[0].m.p, p.ulhc.m.c},
				r:  dot{cs[0].r.p, p.ulhc.r.c},
				rr: dot{cs[0].rr.p, p.ulhc.rr.c},
			}
			p.urhc = corner{
				ll: dot{cs[1].ll.p, p.urhc.ll.c},
				l:  dot{cs[1].l.p, p.urhc.l.c},
				m:  dot{cs[1].m.p, p.urhc.m.c},
				r:  dot{cs[1].r.p, p.urhc.r.c},
				rr: dot{cs[1].rr.p, p.urhc.rr.c},
			}
			p.lrhc = corner{
				ll: dot{cs[2].ll.p, p.lrhc.ll.c},
				l:  dot{cs[2].l.p, p.lrhc.l.c},
				m:  dot{cs[2].m.p, p.lrhc.m.c},
				r:  dot{cs[2].r.p, p.lrhc.r.c},
				rr: dot{cs[2].rr.p, p.lrhc.rr.c},
			}
			p.llhc = corner{
				ll: dot{cs[3].ll.p, p.llhc.ll.c},
				l:  dot{cs[3].l.p, p.llhc.l.c},
				m:  dot{cs[3].m.p, p.llhc.m.c},
				r:  dot{cs[3].r.p, p.llhc.r.c},
				rr: dot{cs[3].rr.p, p.llhc.rr.c},
			}

			rightArm := p.ulhc.rr.p.sub(p.ulhc.m.p)
			rightAbs := p.ulhc.m.p.add(point{100, 0}).sub(p.ulhc.m.p)
			angle := angleBetween(rightArm, rightAbs)
			if p.ulhc.rr.p.y < p.ulhc.m.p.y {
				angle = 2*math.Pi - angle
			}
			p.angle = angle
			pages[p.id] = p

			// persist this page across frames
			pagePersist := persistPage{id: p.id, ttl: 10}
			persistCorners[p.ulhc] = pagePersist
			persistCorners[p.urhc] = pagePersist
			persistCorners[p.lrhc] = pagePersist
			persistCorners[p.llhc] = pagePersist

			// Clockwise from upper left hand corner
			pts := []point{p.ulhc.m.p, p.urhc.m.p, p.lrhc.m.p, p.llhc.m.p}
			center := pts[0].add(pts[1]).add(pts[2]).add(pts[3]).div(4)
			r := ptsToRect([]point{
				rotateAround(center, pts[0], angle),
				rotateAround(center, pts[1], angle),
				rotateAround(center, pts[2], angle),
				rotateAround(center, pts[3], angle),
			})
			gocv.Rectangle(&img, r, green, 2)

			aabb := ptsToRect(pts)
			gocv.Rectangle(&img, aabb, blue, 2)

			// in lisp we store the points already translated to beamerspace instead of webcamspace
			// NOTE: this means distances between papers in inches should use a conversion as well!
			//for i, pt := range pts {
			//pts[i] = translate(pt, cResults.displacement, cResults.displayRatio)
			projPoints := gocv.NewPoint2fVector()
			defer projPoints.Close()
			jacobian := gocv.NewMat()
			defer jacobian.Close()
			ptsZ0 := []gocv.Point3f{}
			for _, pt := range pts {
				ptsZ0 = append(ptsZ0, gocv.Point3f{float32(pt.x), float32(pt.y), 0})
			}
			//objPts := gocv.NewPoint3fVectorFromPoints(ptsZ0)
			//gocv.ProjectPoints(objPts, cResults.rvec, cResults.tvec, cResults.mtx, cResults.dist, projPoints, &jacobian, 0)
			for i, pt := range projPoints.ToPoints() {
				pts[i] = point{float64(pt.X), float64(pt.Y)}
			}
			//}

			dID := page2lisp(l, p, pts)
			datalogIDs[p.id] = dID

			fmt.Println("Found page with ID ", p.id)
		}

		evalPages(l, pages, datalogIDs)

		for _, illu := range opencv.Illus {
			blit(&illu, &cimg)
			illu.Close()
		}
		opencv.Illus = []gocv.Mat{}

	}, 10); err != nil {
		fmt.Println(err)
	}
}

// TODO: only works if area to be colored is still black
// smth like 'set nonblack area in 'from' to white, use that as mask, blacken 'to' area with mask first?'
func blit(from, to *gocv.Mat) {
	gocv.BitwiseOr(*from, *to, to)
}
