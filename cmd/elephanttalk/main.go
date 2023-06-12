package main

import (
	_ "embed"

	"github.com/deosjr/elephanttalk/talk"
)

//go:embed test.lisp
var testpage string

func main() {
	// instead of using all coloured dots to identify pages, only use the corner dots
	talk.UseSimplifiedIDs()

	//page1
	//talk.AddPageFromShorthand("ygybr", "brgry", "gbgyg", "bgryy", `(claim this 'outlined 'blue)`)
	talk.AddPageFromShorthand("ygybr", "brgry", "gbgyg", "bgryy", `(claim this 'pointing 30)`)

	//page2
	talk.AddPageFromShorthand("yggyg", "rgyrb", "bybbg", "brgrg", `(claim this 'highlighted 'red)`)

	//page that always counts as recognised but doesnt have to be present physically
	talk.AddBackgroundPage(testpage)

	talk.Run()
}
