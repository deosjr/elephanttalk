package main

import (
	"github.com/deosjr/elephanttalk/talk"
)

func main() {

	//page1
	talk.AddPageFromShorthand("ygybr", "brgry", "gbgyg", "bgryy", `(claim this 'outlined 'blue)`)

	//page2
	talk.AddPageFromShorthand("yggyg", "rgyrb", "bybbg", "brgrg", `(claim this 'highlighted 'red)`)

	talk.Run()
}
