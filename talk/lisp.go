package talk

import (
	_ "embed"
	"fmt"

	"github.com/deosjr/elephanttalk/northvolt"
	"github.com/deosjr/elephanttalk/opencv"
	"github.com/deosjr/whistle/datalog"
	"github.com/deosjr/whistle/kanren"
	"github.com/deosjr/whistle/lisp"
	"github.com/northvolt/graphql-schema/model"
)

//go:embed talk.lisp
var elephanttalk string

func LoadRealTalk() lisp.Lisp {
	l := lisp.New()
	kanren.Load(l)
	datalog.Load(l)
	if err := l.Load(elephanttalk); err != nil {
		panic(err)
	}
	opencv.Load(l.Env)
	northvolt.Load(l.Env)
	id := "c-001697947722"
	out, err := l.Eval(fmt.Sprintf("(dt:identity %q)", id))
	if err != nil {
		fmt.Println(err)
	} else {
		identity := out.AsPrimitive().(model.NorthvoltIdentity)
		fmt.Println(identity)
	}
	return l
}

// clear datalog db global vars at start of each frame
func clear(l lisp.Lisp) {
	l.Eval("(set! dl_edb (make-hashmap))")
	l.Eval("(set! dl_idb (make-hashmap))")
	l.Eval("(set! dl_rdb (quote ()))")
	l.Eval("(set! dl_idx_entity (make-hashmap))")
	l.Eval("(set! dl_idx_attr (make-hashmap))")
	l.Eval("(set! dl_counter 0)")
}

// write a recognised page to lisp, storing it in datalog
// returns an int identifier for this page, which is unique in this frame only
func page2lisp(l lisp.Lisp, p page, pts []point) int {
	lisppoints := fmt.Sprintf("(list (cons %f %f) (cons %f %f) (cons %f %f) (cons %f %f))", pts[0].x, pts[0].y, pts[1].x, pts[1].y, pts[2].x, pts[2].y, pts[3].x, pts[3].y)
	dID, _ := l.Eval(fmt.Sprintf(`(dl_record 'page
        ('id %d)
        ('points %s)
        ('angle %f)
        ('code %q)
    )`, p.id, lisppoints, p.angle, p.code))
	return int(dID.AsNumber())
}

func evalPages(l lisp.Lisp, pages map[uint64]page, datalogIDs map[uint64]int) {
	for _, page := range backgroundPages {
		_, err := l.Eval(page.code)
		if err != nil {
			fmt.Println(page.id, err)
		}
	}

	for _, page := range pages {
		// v1 of claim/wish/when
		// run each pages' code, including claims, wishes and whens
		// set 'this to the page's id
		_, err := l.Eval(fmt.Sprintf("(define this %d)", datalogIDs[page.id]))
		if err != nil {
			fmt.Println(err)
		}
		_, err = l.Eval(page.code)
		if err != nil {
			fmt.Println(page.id, err)
		}
	}

	_, err := l.Eval("(dl_fixpoint)")
	if err != nil {
		fmt.Println("fixpoint", err)
	}
}
