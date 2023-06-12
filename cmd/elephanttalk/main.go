package main

import (
	_ "embed"
	"log"

	"github.com/deosjr/elephanttalk/talk"
)

//go:embed test.lisp
var testpage string

func main() {
	log.SetFlags(0)
	log.Print("Starting...")

	// instead of using all coloured dots to identify pages, only use the corner dots
	talk.UseSimplifiedIDs()

	//page1
	//talk.AddPageFromShorthand("ygybr", "brgry", "gbgyg", "bgryy", `(claim this 'outlined 'blue)`)
	talk.AddPageFromShorthand("ygybr", "brgry", "gbgyg", "bgryy", `(claim this 'pointing 30)`)

	//page2
	// talk.AddPageFromShorthand("yggyg", "rgyrb", "bybbg", "brgrg", `(claim this 'highlighted 'red)`)
	talk.AddPageFromShorthand("yggyg", "rgyrb", "bybbg", "brgrg", `(claim this 'outlined 'blue)`)

	//page that always counts as recognised but doesnt have to be present physically
	talk.AddBackgroundPage(testpage)

	talk.Run()
}
