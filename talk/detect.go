package talk

import (
	"image"
	"image/color"
	"math"
	"sort"

	"gocv.io/x/gocv"
)

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
			/*
				if ref != nil {
					closeEnough := false
					for _, refC := range ref {
						if colorDistance(c, refC) < 1000000 {
							closeEnough = true
						}
					}
					if !closeEnough {
						continue
					}
				}
			*/

			mid := image.Pt(int(x), int(y))
			for rect, list := range spatialPartition {
				if mid.In(rect) {
					spatialPartition[rect] = append(list, circle{point{x, y}, r, c})
				}
			}

			gocv.Circle(&img, mid, int(r), color.RGBA{0, 0, 255, 0}, 2)
			gocv.Circle(&img, mid, 2, color.RGBA{255, 0, 0, 0}, 3)
		}
	}
	return spatialPartition
}

// calibration pattern is four circles in a rectangle
// check if they are equidistant to their midpoint
func findCalibrationPattern(v []circle) bool {
	if len(v) != 4 {
		return false
	}
	midpoint := circlesMidpoint(v)

	dist := euclidian(midpoint.sub(v[0].mid))
	for _, p := range v[1:] {
		ddist := euclidian(midpoint.sub(p.mid))
		if !equalWithMargin(ddist, dist, 2.0) {
			return false
		}
	}
	return true
}

func findCorners(v []circle, ref []color.RGBA) (corner, bool) {
	// first detect lines
	lines := [][]circle{}
	for i, c := range v {
		dists := map[int][]int{}
		for j, o := range v {
			if i == j {
				continue
			}
			// magic number? bucketing distances is hard
			d := int(euclidian(c.mid.sub(o.mid)) / 10)
			dists[d] = append(dists[d], j)
		}
		var candidate []int
		for _, indices := range dists {
			if len(indices) == 2 {
				candidate = indices
				break
			}
		}
		if candidate == nil {
			continue
		}
		line1 := v[candidate[0]].mid.sub(c.mid)
		line2 := v[candidate[1]].mid.sub(c.mid)
		dot := line1.x*line2.x + line1.y*line2.y
		angle := math.Acos(dot / (euclidian(line1) * euclidian(line2)))
		epsilon := math.Abs(angle - math.Pi)
		if epsilon < 0.2 {
			lines = append(lines, []circle{v[candidate[0]], c, v[candidate[1]]})
		}
	}
	if len(lines) != 2 {
		return corner{}, false
	}

	line1 := lines[0]
	line2 := lines[1]
	var top, end1, end2 circle
	switch {
	case line1[0] == line2[0]:
		top = line1[0]
		end1, end2 = line1[2], line2[2]
	case line1[2] == line2[2]:
		top = line1[2]
		end1, end2 = line1[0], line2[0]
	case line1[0] == line2[2]:
		top = line1[0]
		end1, end2 = line1[2], line2[0]
	case line1[2] == line2[0]:
		top = line1[2]
		end1, end2 = line1[0], line2[2]
	default:
		return corner{}, false
	}

	mid1, mid2 := line1[1], line2[1]
	v = []circle{end1, mid1, top, mid2, end2}

	// midpoint test
	midpoint := circlesMidpoint(v)

	sortedDistances := []float64{}
	for _, p := range v {
		sortedDistances = append(sortedDistances, euclidian(midpoint.sub(p.mid)))
	}
	sort.Float64s(sortedDistances)
	// first 3 are roughly equal, last 2 are roughly x2
	// middle one is the 'top' of the 'arrow'
	first3 := (sortedDistances[0] + sortedDistances[1] + sortedDistances[2]) / 3.0
	last2 := (sortedDistances[3] + sortedDistances[4]) / 2.0
	if !equalWithMargin(first3*2, last2, 5.0) {
		return corner{}, false
	}
	if !equalWithMargin(sortedDistances[0], sortedDistances[1], 3.0) {
		return corner{}, false
	}
	if !equalWithMargin(sortedDistances[0], sortedDistances[2], 6.0) {
		return corner{}, false
	}
	if !equalWithMargin(sortedDistances[1], sortedDistances[2], 6.0) {
		return corner{}, false
	}
	if !equalWithMargin(sortedDistances[3], sortedDistances[4], 6.0) {
		return corner{}, false
	}

	// Rotate both ends around top by a quarter. One ends on top of the other: this is _left_
	rot1 := rotateAround(top.mid, end1.mid, math.Pi/2.)
	rot2 := rotateAround(top.mid, end2.mid, math.Pi/2.)

	var left, leftmid, right, rightmid circle

	if euclidian(rot1.sub(end2.mid)) < 10 {
		left = end1
		leftmid = mid1
		rightmid = mid2
		right = end2
	} else if euclidian(rot2.sub(end1.mid)) < 10 {
		left = end2
		leftmid = mid2
		rightmid = mid1
		right = end1
	} else {
		return corner{}, false
	}

	v = []circle{left, leftmid, top, rightmid, right}

	colors := make([]dotColor, 5)
	for i, c := range v {
		sample := c.c
		dist := math.MaxFloat64
		for j, refC := range ref {
			if d := colorDistance(sample, refC); d < dist {
				dist = d
				colors[i] = dotColor(j)
			}
		}
		rr, gg, bb, _ := sample.RGBA()
		rr = rr >> 8
		gg = gg >> 8
		bb = bb >> 8
		switch {
		case rr < 80 && gg < 80 && bb < 80:
			colors[i] = blueDot
		case gg > rr && gg > bb:
			colors[i] = greenDot
		case rr > 2*gg && gg > bb+20:
			colors[i] = yellowDot
		case rr > 2*gg && rr > 3*bb:
			colors[i] = redDot
		}
	}
	return corner{
		ll: dot{p: left.mid, c: colors[0]},
		l:  dot{p: leftmid.mid, c: colors[1]},
		m:  dot{p: top.mid, c: colors[2]},
		r:  dot{p: rightmid.mid, c: colors[3]},
		rr: dot{p: right.mid, c: colors[4]},
	}, true
}

func equalWithMargin(x, y, margin float64) bool {
	return !(x-margin > y || x+margin < y)
}
