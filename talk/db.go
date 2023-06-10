package talk

import (
	"strings"
)

// TODO: a proper database solution, inmem is good enough for now
var pageDB = map[uint32]page{}

var backgroundPages = []page{}

// AddBackgroundPage adds a virtual page to the db which always counts as recognised in a frame.
// This means its code will always get executed. 'this' is not supported since pageID doesnt have strong guarantees
func AddBackgroundPage(code string) {
	backgroundPages = append(backgroundPages, page{code: code})
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

// Each 3 consecutive corners have their own partial ID
// We store all 4 of those for each page, and each has to be unique!
// This allows us to find a page with only 3 corners detected
func addToDB(p page) bool {
	id1 := pagePartialID(p.ulhc.id(), p.urhc.id(), p.lrhc.id())
	id2 := pagePartialID(p.urhc.id(), p.lrhc.id(), p.llhc.id())
	id3 := pagePartialID(p.lrhc.id(), p.llhc.id(), p.ulhc.id())
	id4 := pagePartialID(p.llhc.id(), p.ulhc.id(), p.urhc.id())
	if _, ok := pageDB[id1]; ok {
		return false
	}
	if _, ok := pageDB[id2]; ok {
		return false
	}
	if _, ok := pageDB[id3]; ok {
		return false
	}
	if _, ok := pageDB[id4]; ok {
		return false
	}
	p.id = pageID(p.ulhc.id(), p.urhc.id(), p.lrhc.id(), p.llhc.id())
	pageDB[id1] = p
	pageDB[id2] = p
	pageDB[id3] = p
	pageDB[id4] = p
	return true
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
