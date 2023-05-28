package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/deosjr/lispadventures/lisp"
	"gocv.io/x/gocv"
)

type frameInput struct {
	webcam      *gocv.VideoCapture
	debugWindow *gocv.Window
	projection  *gocv.Window
	// TODO: should these be passed as ptrs?
	img  gocv.Mat
	cimg gocv.Mat
}

func frameloop(fi frameInput, f func(image.Image, map[image.Rectangle][]circle), ref []color.RGBA, waitMillis int) error {
	for {
		start := time.Now()
		if ok := fi.webcam.Read(&fi.img); !ok {
			return fmt.Errorf("cannot read device\n")
		}
		if fi.img.Empty() {
			continue
		}
		// since detect draws in img, we take a snapshot first
		actualImage, _ := fi.img.ToImage()
		spatialPartition := detect(fi.img, actualImage, ref)

		f(actualImage, spatialPartition)

		fps := time.Second / time.Since(start)
		gocv.PutText(&fi.img, fmt.Sprintf("FPS: %d", fps), image.Pt(0, 20), 0, .5, color.RGBA{}, 2)

		fi.debugWindow.IMShow(fi.img)
		fi.projection.IMShow(fi.cimg)
		key := fi.debugWindow.WaitKey(waitMillis)
		if key >= 0 {
			fmt.Println(key)
			return nil
		}
	}
}

func detect(img gocv.Mat, actualImage image.Image, ref []color.RGBA) map[image.Rectangle][]circle {
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
		float64(img.Rows()/64), // minDistance between centers
		75,                     // param1
		20,                     // param2
		1,                      // minRadius
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

	for i := 0; i < circleMat.Cols(); i++ {
		v := circleMat.GetVecfAt(0, i)
		// if circles are found
		if len(v) > 2 {
			x := float64(v[0])
			y := float64(v[1])
			r := float64(v[2])

			c := actualImage.At(int(x), int(y))
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

			mid := image.Pt(int(x), int(y))
			for rect, list := range spatialPartition {
				if mid.In(rect) {
					spatialPartition[rect] = append(list, circle{point{x,y}, r, c})
				}
			}

			gocv.Circle(&img, mid, int(r), color.RGBA{0, 0, 255, 0}, 2)
			gocv.Circle(&img, mid, 2, color.RGBA{255, 0, 0, 0}, 3)
		}
	}
	return spatialPartition
}

func vision(webcam *gocv.VideoCapture, debugwindow, projection *gocv.Window, cResults calibrationResults) {
	img := gocv.NewMat()
	defer img.Close()
	cimg := gocv.NewMatWithSize(beamerHeight, beamerWidth, gocv.MatTypeCV8UC3)
	defer cimg.Close()

	l := lisp.New()
	LoadRealTalk(l)

	fi := frameInput{
		webcam:      webcam,
		debugWindow: debugwindow,
		projection:  projection,
		img:         img,
		cimg:        cimg,
	}

	if err := frameloop(fi, func(_ image.Image, spatialPartition map[image.Rectangle][]circle) {
		// clear datalog dbs
		l.Eval("(set! dl_edb (make-hashmap))")
		l.Eval("(set! dl_idb (make-hashmap))")
		l.Eval("(set! dl_rdb (quote ()))")
		l.Eval("(set! dl_idx_entity (make-hashmap))")
		l.Eval("(set! dl_idx_attr (make-hashmap))")
		l.Eval("(set! dl_counter 0)")
		pageDatalogIDs := map[uint64]int{}
		pageIDsDatalog := map[int]uint64{}

		gocv.Circle(&img, image.Pt(5, 5), 5, cResults.referenceColors[0], -1)
		gocv.Circle(&img, image.Pt(15, 5), 5, cResults.referenceColors[1], -1)
		gocv.Circle(&img, image.Pt(25, 5), 5, cResults.referenceColors[2], -1)
		gocv.Circle(&img, image.Pt(35, 5), 5, cResults.referenceColors[3], -1)

		red := color.RGBA{255, 0, 0, 0}
		green := color.RGBA{0, 255, 0, 0}
		blue := color.RGBA{0, 0, 255, 0}
		yellow := color.RGBA{255, 255, 0, 0}

		gocv.Rectangle(&cimg, image.Rect(0, 0, beamerWidth, beamerHeight), color.RGBA{}, -1)

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
			gocv.Line(&img, corner.m.p.toIntPt(), corner.ll.p.toIntPt(), blue, 2)
			gocv.Line(&img, corner.m.p.toIntPt(), corner.rr.p.toIntPt(), blue, 2)
			//gocv.PutText(&img, fmt.Sprintf("%010b", corner.id()), corner.m.p.Add(image.Pt(10, 40)), 0, .5, color.RGBA{}, 2)

			// calculate angle between right arm of corner and absolute right in webcam space
			rightArm := corner.rr.p.sub(corner.m.p)
			rightAbs := corner.m.p.add(point{100, 0}).sub(corner.m.p)
			angle := angleBetween(rightArm, rightAbs)
			if corner.rr.p.y < corner.m.p.y {
				angle = 2*math.Pi - angle
			}
			gocv.PutText(&img, fmt.Sprintf("%f", angle), corner.m.p.add(point{10, 20}).toIntPt(), 0, .5, color.RGBA{}, 2)

			cs := []color.RGBA{red, green, blue, yellow}
			gocv.Circle(&img, corner.ll.p.toIntPt(), 8, cs[int(corner.ll.c)], -1)
			gocv.Circle(&img, corner.l.p.toIntPt(), 8, cs[int(corner.l.c)], -1)
			gocv.Circle(&img, corner.m.p.toIntPt(), 8, cs[int(corner.m.c)], -1)
			gocv.Circle(&img, corner.r.p.toIntPt(), 8, cs[int(corner.r.c)], -1)
			gocv.Circle(&img, corner.rr.p.toIntPt(), 8, cs[int(corner.rr.c)], -1)

			cornersByTop[corner.m.p.toIntPt()] = corner
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
				right := c.rr.p.sub(c.m.p)
				toO := o.m.p.sub(c.m.p)
				angle1 := angleBetween(right, toO)
				left := o.ll.p.sub(o.m.p)
				toC := c.m.p.sub(o.m.p)
				angle2 := angleBetween(left, toC)
				if angle1 > 0.05 || angle2 > 0.05 {
					continue
				}
				prev, ok := cornerMap[c]
				if ok {
					// overwrite previously found corner if this one is closer
					if euclidian(c.m.p.sub(prev.m.p)) > euclidian(c.m.p.sub(o.m.p)) {
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
			// because cs[0] = cs[4], remove one instance of that corner
			cs = cs[:4]
			sortCorners(cs)
			// naive: shift up to 4 times to try and find a valid page
			p := page{ulhc: cs[0], urhc: cs[1], llhc: cs[2], lrhc: cs[3]}
			for i := 0; i < 4; i++ {
				pID := pageID(p.ulhc.id(), p.urhc.id(), p.lrhc.id(), p.llhc.id())
				p.id = pID
				pp, ok := pageDB[pID]
				if !ok {
					p.ulhc, p.urhc, p.lrhc, p.llhc = p.urhc, p.lrhc, p.llhc, p.ulhc
					continue
				}
				p.code = pp.code
				rightArm := p.ulhc.rr.p.sub(p.ulhc.m.p)
				rightAbs := p.ulhc.m.p.add(point{100, 0}).sub(p.ulhc.m.p)
				angle := angleBetween(rightArm, rightAbs)
				if p.ulhc.rr.p.y < p.ulhc.m.p.y {
					angle = 2*math.Pi - angle
				}
				p.angle = angle
				pages = append(pages, p)

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
                for i, pt := range pts {
                    pts[i] = translate(pt, cResults.displacement, cResults.displayRatio)
                }

				lisppoints := fmt.Sprintf("(list (cons %f %f) (cons %f %f) (cons %f %f) (cons %f %f))", pts[0].x, pts[0].y, pts[1].x, pts[1].y, pts[2].x, pts[2].y, pts[3].x, pts[3].y)
				dlID, _ := l.Eval(fmt.Sprintf(`(dl_record 'page
                    ('id %d)
                    ('points %s)
                    ('angle %f)
                    ('code %q)
                )`, p.id, lisppoints, p.angle, p.code))
				pageDatalogIDs[pID] = int(dlID.AsNumber())
				pageIDsDatalog[int(dlID.AsNumber())] = pID
				break
			}
		}

        // TODO for testing purposes, this page always counts as recognized
        // TODO: add back possibility to write text under rotation
        testpage := page{
            id: 42,
            code: `(begin
            (-- should be counterclockwise, somehow isnt; fixed for now by negating angle --)
            (define rotateAround (lambda (pivot point angle)
              (let ((s (sin (- 0 angle)))
                    (c (cos (- 0 angle)))
                    (px (car point))
                    (py (cdr point))
                    (cx (car pivot))
                    (cy (cdr pivot)))
                (let ((x (- px cx))
                      (y (- py cy)))
                  (cons
                    (+ cx (- (* c x) (* s y)))
                    (+ cy (+ (* s x) (* c y))))))))

            (define point-add (lambda (p q)
              (cons
                (+ (car p) (car q))
                (+ (cdr p) (cdr q)))))

            (define point-div (lambda (p n)
              (cons (/ (car p) n) (/ (cdr p) n))))

            (define midpoint (lambda (points)
              (point-div (foldl point-add points (cons 0 0)) (length points))))

            (define points->rect (lambda (points)
              (let ((rects (map (lambda (p)
                (let ((min (point-add p (cons -1 -1))) (max (point-add p (cons 1 1))))
                  (make-rectangle (car min) (cdr min) (car max) (cdr max)))) points)))
                    (-- (foldl rects rect:union (car rects)) --)
                    (rect:union (rect:union (rect:union (car rects) (car (cdr rects))) (car (cdr (cdr rects)))) (car (cdr (cdr (cdr rects)))))
                   )))

            (-- TODO: illu (ie gocv.Mat) is not hashable, so cant store it in claim in db. pass by ref? --)

            (when ((highlighted ,?page ,?color) ((page points) ,?page ,?points) ((page angle) ,?page ,?angle)) do
                (let ((center (midpoint (quote ,?points)))
                      (unangle (* -360 (/ ,?angle (* 2 pi))))
                      (illu (make-illumination)))
                  (let ((rotated (map (lambda (p) (rotateAround center p ,?angle)) (quote ,?points)))
                        (m (gocv:rotation_matrix2D (car center) (cdr center) unangle 1.0)))
                    (gocv:rect illu (points->rect rotated) ,?color -1)
                    (-- might not work because it doesnt support inplace --)
                    (gocv:warp_affine illu illu m 1280 720)
                    (claim ,?page 'has-illumination 'illu))
                ))
            )`,
        }
        pageDB[42] = testpage
        pageDatalogIDs[42] = 0
        pageIDsDatalog[0] = 42
        pages = append(pages, testpage)

		for _, page := range pages {
			// v1 of claim/wish/when
			// run each pages' code, including claims, wishes and whens
			// set 'this to the page's id
			_, err := l.Eval(fmt.Sprintf("(define this %d)", pageDatalogIDs[page.id]))
			if err != nil {
				fmt.Println(err)
			}
			_, err = l.Eval(page.code)
			if err != nil {
				fmt.Println(page.id, err)
			}
		}

		_, err := l.Eval("(dl_fixpoint)")
		if err != nil {
			fmt.Println("fixpoint", err)
		}

		ids, err := l.Eval("(dl_find ,?id where ((,?id has-illumination ,?illu)))")
		if err != nil {
			fmt.Println("find", err)
		}

		highlightIDs := map[uint64]struct{}{}
		lids, _ := lisp.UnpackConsList(ids)
		for _, idprim := range lids {
			id := int(idprim.AsNumber())
			highlightIDs[pageIDsDatalog[id]] = struct{}{}
		}

        for _, illu := range illus {
			blit(&illu, &cimg)
            illu.Close()
        }
        illus = []gocv.Mat{}

	}, cResults.referenceColors, 10); err != nil {
		fmt.Println(err)
	}
}

// TODO: only works if area to be colored is still black
// smth like 'set nonblack area in 'from' to white, use that as mask, blacken 'to' area with mask first?'
func blit(from, to *gocv.Mat) {
	gocv.BitwiseOr(*from, *to, to)
}
