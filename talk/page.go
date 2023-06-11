package talk

import (
	"image/color"
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
