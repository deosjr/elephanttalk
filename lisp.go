package main

import (
	"image"
	"image/color"

	"github.com/deosjr/lispadventures/lisp"
	"gocv.io/x/gocv"
)

// v2 version of claim/wish/when model
// samples (claims/wishes) are no longer the same: a claim is a hard assert into db,
// but a wish is a special kind of assertion to be used in 'when'
// example: when /someone/ wishes x: <code>
// after running fixpoint analysis once per frame, some claims are still picked up
// outside of db and executed upon, mostly illumination-related (blit)
// TODO: insertion of var 'this' does not work properly? execution context is not correct
// solution: insert (define this ?id) at start of each codeblock?
func LoadRealTalk(l lisp.Lisp) {
	l.Eval(`(define-syntax claim
              (syntax-rules (dl_assert this claims list)
                ((_ id attr value) (begin
                 (dl_assert this 'claims (list id attr value))
                 (dl_assert id attr value)))))`)

	l.Eval(`(define-syntax wish
              (syntax-rules (dl_assert this wishes)
                ((_ x) (dl_assert this 'wishes (quote x)))))`)
	// 'when' makes a rule and includes code execution
	// this code execution is handled by hacking into the datalog implementation (see below)
	// code can include further claims/wishes or even other when-statements
	// NOTE: for now, this is going to be executing on every fixpoint iteration that matches,
	// so it better be idempotent / not too inefficient!
	// if conditions match, assert a fact (?id 'code ?code) where ?code already has vars replaced
	// (when (is-a (unquote ?page) window) do (wish ((unquote ?page) highlighted blue)))
	// TODO: multiple conditions, because conflicts with 'do' currently (?)
	l.Eval(`(define-syntax when
              (syntax-rules (wishes do code this dl_rule :- begin)
                ((_ condition do statement ...)
                 (dl_rule (code this (begin statement ...)) :- condition))
                ((_ someone wishes w do statement ...)
                 (dl_rule (code this (begin statement ...)) :- (wishes someone w)))))`)

	// overwrite part of datalog naive fixpoint implementation
	// to include code execution in when-blocks!
	// NOTE: assumes all rules are ((code id (stmt ...)) :- condition ...)
	// runs each newly found code to run using map eval
	// NOTE: order is _not_ guaranteed but once code includes bindings, so same rule should only run once per set of bindings
	// (due to key equivalence being checked on the FULL sexpression code)
	// TODO: do we even need to update indices?
	l.Eval(`(define dl_fixpoint_iterate (lambda ()
       (let ((new (hashmap-keys (set_difference (foldl (lambda (x y) (set-extend! y x)) (map dl_apply_rule dl_rdb) (make-hashmap)) dl_idb))))
         (set-extend! dl_idb new)
         (map dl_update_indices new)
         (map (lambda (c) (eval (car (cdr (cdr c))))) new)
         (if (not (null? new)) (dl_fixpoint_iterate)))))`)

	loadGoCV(l.Env)
}

// wrapper around gocv for use in lisp code

func loadGoCV(env *lisp.Env) {
	// colors
	env.Add("black", lisp.NewPrimitive(color.RGBA{}))
	env.Add("white", lisp.NewPrimitive(color.RGBA{255, 255, 255, 0}))
	env.Add("red", lisp.NewPrimitive(color.RGBA{255, 0, 0, 0}))
	env.Add("green", lisp.NewPrimitive(color.RGBA{0, 255, 0, 0}))
	env.Add("blue", lisp.NewPrimitive(color.RGBA{0, 0, 255, 0}))

	// illumination is a gocv mat
	env.AddBuiltin("new_illumination", newIllumination)
	//env.AddBuiltin("ill:rectangle", )

	// golang image lib for 2d primitives
	// TODO: if we reason only in projector space or even page space
	// some of this might become less relevant or even confusing
	env.AddBuiltin("point2d", newPoint2D)
	env.AddBuiltin("cons2point", cons2point)
	env.AddBuiltin("new_rectangle", newRectangle)
}

// TODO: defer close? memory leak otherwise?
// dont want to write close in lisp, so for now we'll need some memory mngment
// outside of it (ie keeping track in Go of created mats and closing them)
var illus = []gocv.Mat{}

func newIllumination(args []lisp.SExpression) (lisp.SExpression, error) {
	illu := gocv.NewMatWithSize(1280, 720, gocv.MatTypeCV8UC3)
	illus = append(illus, illu)
	return lisp.NewPrimitive(illu), nil
}

// (point2D x y) -> image.Point primitive
func newPoint2D(args []lisp.SExpression) (lisp.SExpression, error) {
	x, y := int(args[0].AsNumber()), int(args[1].AsNumber())
	return lisp.NewPrimitive(image.Pt(x, y)), nil
}

// (cons2point (x y)) -> image.Point primitive
func cons2point(args []lisp.SExpression) (lisp.SExpression, error) {
	c, _ := lisp.UnpackConsList(args[0])
	x, y := int(c[0].AsNumber()), int(c[1].AsNumber())
	return lisp.NewPrimitive(image.Pt(x, y)), nil
}

// (new_rectangle min:image.Point max:image.Point) -> image.Rectangle primitive
func newRectangle(args []lisp.SExpression) (lisp.SExpression, error) {
	min, max := args[0].AsPrimitive().(image.Point), args[1].AsPrimitive().(image.Point)
	r := image.Rectangle{min, max}
	return lisp.NewPrimitive(r), nil
}
