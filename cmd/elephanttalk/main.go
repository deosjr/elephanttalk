package main

import (
	_ "embed"

	"github.com/deosjr/elephanttalk/talk"
)

//go:embed test.lisp
var testpage string

func main() {
	// talk.PrintCalibrationPage()
	// talk.PrintPageFromShorthand("ygybr", "brgry", "gbgyg", "bgryy", `(claim this 'outlined 'blue)`)

	// instead of using all coloured dots to identify pages, only use the corner dots
	// talk.UseSimplifiedIDs()

	//page1
	talk.AddPageFromShorthand("bbrrg", "rybbg", "brbyy", "rybgg", `(claim this 'outlined 'blue)`)
	//talk.AddPageFromShorthand("ygybr", "brgry", "gbgyg", "bgryy", `(claim this 'pointing 30)`)

	//page2
	talk.AddPageFromShorthand("bgrrb", "rybgg", "rbryg", "brgrb", `(claim this 'highlighted 'red)`)

	//testpage
	//talk.AddPageFromShorthand("ybgyr", "ybrgr", "yrgrb", "brygg", `(claim this 'highlighted 'red)`)
	// no yellow, yellow works least well
	talk.AddPageFromShorthand("bgrgb", "rbgbr", "grgrb", "bbrgg", `(claim this 'highlighted 'red)`)
	talk.PrintPageFromShorthand("bgrgb", "rbgbr", "grgrb", "bbrgg", `(claim this 'highlighted 'red)`)

	//page that always counts as recognised but doesnt have to be present physically
	talk.AddBackgroundPage(testpage)

	talk.Run()
}
