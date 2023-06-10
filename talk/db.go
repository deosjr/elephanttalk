package talk

// TODO: a proper database solution, inmem is good enough for now
var pageDB = map[uint32]page{}

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
