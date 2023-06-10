package talk

import (
	_ "embed"

	"github.com/deosjr/elephanttalk/opencv"
	"github.com/deosjr/whistle/lisp"
)

//go:embed talk.lisp
var elephanttalk string

func LoadRealTalk(l lisp.Lisp) {
	if err := l.Load(elephanttalk); err != nil {
		panic(err)
	}
	opencv.Load(l.Env)
}
