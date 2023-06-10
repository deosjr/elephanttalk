package talk

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"sort"
    "strings"
	"time"

	"gocv.io/x/gocv"
)

// using CIELAB color picker and comparing with reference material from dynamicland
// red6, green7, purple8, orange6
var cielabRed = color.RGBA{245, 34, 45, 0}
var cielabGreen = color.RGBA{56, 158, 13, 0}
var cielabBlue = color.RGBA{57, 16, 133, 0}
var cielabYellow = color.RGBA{250, 140, 22, 0}

type page struct {
	id                     uint64
	ulhc, urhc, lrhc, llhc corner
	angle                  float64
	code                   string
}

// AddPageFromShorthand lets you add a page to the database when you already know its corners
// used while the database doesnt persist across sessions, so we dont print new pages all the time
func AddPageFromShorthand(ulhc, urhc, lrhc, llhc, code string) bool {
    return addToDB(page{
        ulhc: cornerShorthand(ulhc),
        urhc: cornerShorthand(urhc),
        llhc: cornerShorthand(llhc),
        lrhc: cornerShorthand(lrhc),
        code: code,
    })
}

// to define left and right under rotation:
// left arm of the corner can make a 90 degree counterclockwise rotation
// and end up on top of the right arm, 'closing' the corner
type corner struct {
	ll, l, m, r, rr dot
}

func (c corner) debugPrint() string {
	s := []string{"r", "g", "b", "y"}
	return s[c.ll.c] + s[c.l.c] + s[c.m.c] + s[c.r.c] + s[c.rr.c]
}

// each dotColor stores 2 bits of info
// one corner therefore has 10 bits of information
func (c corner) id() uint16 {
	var out uint16
	out |= uint16(c.rr.c)
	out |= uint16(c.r.c) << 2
	out |= uint16(c.m.c) << 4
	out |= uint16(c.l.c) << 6
	out |= uint16(c.ll.c) << 8
	return out
}

// one page has 4 corners, therefore a 40 bit unique id in theory
// however, we want to still recognise a paper when one corner is covered
// practically this means each paper has 4 unique 30 bit ids (related by a 10-bit shift)
// this takes 2 bits out of the space of unique 30 bit pageIDs, so 2**28 remain
func pageID(ulhc, urhc, lrhc, llhc uint16) uint64 {
	var out uint64
	out |= uint64(llhc)
	out |= uint64(lrhc) << 10
	out |= uint64(urhc) << 20
	out |= uint64(ulhc) << 30
	return out
}

func pagePartialID(x, y, z uint16) uint32 {
	var out uint32
	out |= uint32(z)
	out |= uint32(y) << 10
	out |= uint32(x) << 20
	return out
}

func cornerShorthand(debug string) corner {
	s := "rgby"
	return corner{
		ll: dot{c: dotColor(strings.IndexRune(s, rune(debug[0])))},
		l:  dot{c: dotColor(strings.IndexRune(s, rune(debug[1])))},
		m:  dot{c: dotColor(strings.IndexRune(s, rune(debug[2])))},
		r:  dot{c: dotColor(strings.IndexRune(s, rune(debug[3])))},
		rr: dot{c: dotColor(strings.IndexRune(s, rune(debug[4])))},
	}
}

type dot struct {
	p point
	c dotColor
}

type dotColor uint8

const (
	redDot dotColor = iota
	greenDot
	blueDot
	yellowDot
)

func equalWithMargin(x, y, margin float64) bool {
	return !(x-margin > y || x+margin < y)
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

func calibrationPage() {
	w, h := 2480, 3508 // 300 ppi/dpi
	// a4 in cm: 21 x 29.7
	// which means 1cm in pixels = 2480/21 =~ 118
	img := gocv.NewMatWithSize(h, w, gocv.MatTypeCV8UC3)
	defer img.Close()

	red := cielabRed
	green := cielabGreen
	blue := cielabBlue
	yellow := cielabYellow
	white := color.RGBA{255, 255, 255, 0}

	midw, midh := w/2., h/2.
	d := int(1.5 * 118) // circle radius = 1, circle distance = 1
	gocv.Rectangle(&img, image.Rect(0, 0, w, h), white, -1)
	gocv.Circle(&img, image.Pt(midw-d, midh-d), 1*118, red, -1)
	gocv.Circle(&img, image.Pt(midw+d, midh-d), 1*118, green, -1)
	gocv.Circle(&img, image.Pt(midw-d, midh+d), 1*118, blue, -1)
	gocv.Circle(&img, image.Pt(midw+d, midh+d), 1*118, yellow, -1)

	gocv.IMWrite("out.png", img)
}

func blank() {
	w, h := 2480, 3508 // 300 ppi/dpi
	// a4 in cm: 21 x 29.7
	// which means 1cm in pixels = 2480/21 =~ 118
	img := gocv.NewMatWithSize(h, w, gocv.MatTypeCV8UC3)
	defer img.Close()

	red := cielabRed
	green := cielabGreen
	blue := cielabBlue
	yellow := cielabYellow
	white := color.RGBA{255, 255, 255, 0}

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	colors := []color.RGBA{red, green, blue, yellow}
	randomColor := func() color.RGBA {
		return colors[rnd.Intn(4)]
	}

	r := 118
	d := r / 2

	gocv.Rectangle(&img, image.Rect(0, 0, w, h), white, -1)

	gocv.Circle(&img, image.Pt(d+r, d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(2*d+3*r, d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(3*d+5*r, d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(d+r, 2*d+3*r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(d+r, 3*d+5*r), r, randomColor(), -1)

	gocv.Circle(&img, image.Pt(w-(d+r), d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(2*d+3*r), d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(3*d+5*r), d+r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(d+r), 2*d+3*r), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(d+r), 3*d+5*r), r, randomColor(), -1)

	gocv.Circle(&img, image.Pt(d+r, h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(2*d+3*r, h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(3*d+5*r, h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(d+r, h-(2*d+3*r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(d+r, h-(3*d+5*r)), r, randomColor(), -1)

	gocv.Circle(&img, image.Pt(w-(d+r), h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(2*d+3*r), h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(3*d+5*r), h-(d+r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(d+r), h-(2*d+3*r)), r, randomColor(), -1)
	gocv.Circle(&img, image.Pt(w-(d+r), h-(3*d+5*r)), r, randomColor(), -1)

	gocv.IMWrite("out.png", img)
}
