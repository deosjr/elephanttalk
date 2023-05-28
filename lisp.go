package main

import (
	"image"
	"image/color"
    "math"

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
	l.Eval(`(define-syntax when
              (syntax-rules (wishes do code this dl_rule :- begin)
                ((_ (condition ...) do statement ...)
                 (dl_rule (code this (begin statement ...)) :- condition ...))
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

    // macro to make comments work
    l.Eval(`(define-syntax --
              (syntax-rules (comment)
                ((_ x ...) (comment (quote x) ...))))`)

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
	env.AddBuiltin("make-illumination", newIllumination)
    // TODO: once we explore declarations in projectionspace vs rotation a bit more
	//env.AddBuiltin("ill:rectangle", illuRectangle)

    // gocv drawing, might be replaced by ill:draw funcs at some point
    env.AddBuiltin("gocv:line", gocvLine)
    env.AddBuiltin("gocv:rect", gocvRectangle)
    env.AddBuiltin("gocv:text", gocvText)
    env.AddBuiltin("gocv:rotation_matrix2D", rotationMatrix)
    env.AddBuiltin("gocv:warp_affine", warpAffine)

	// golang image lib for 2d primitives
	// TODO: if we reason only in projector space or even page space
	// some of this might become less relevant or even confusing
	env.AddBuiltin("point2d", newPoint2D)
	env.AddBuiltin("make-rectangle", newRectangle)
	env.AddBuiltin("rect:union", rectUnion)

    // missing math builtins
    env.AddBuiltin("sin", sine)
    env.AddBuiltin("cos", cosine)

    // comments
    env.AddBuiltin("comment", ignore)
}

// TODO: defer close? memory leak otherwise?
// dont want to write close in lisp, so for now we'll need some memory mngment
// outside of it (ie keeping track in Go of created mats and closing them)
// NOTE: gocv.Mat is unhashable, so we cant even store it in datalog anyways
var illus = []gocv.Mat{}

func newIllumination(args []lisp.SExpression) (lisp.SExpression, error) {
	illu := gocv.NewMatWithSize(beamerHeight, beamerWidth, gocv.MatTypeCV8UC3)
	illus = append(illus, illu)
	return lisp.NewPrimitive(illu), nil
}

// (gocv:line illu p q color fill)
func gocvLine(args []lisp.SExpression) (lisp.SExpression, error) {
    illu := args[0].AsPrimitive().(gocv.Mat)
    p := args[1].AsPrimitive().(image.Point)
    q := args[2].AsPrimitive().(image.Point)
    c := args[3].AsPrimitive().(color.RGBA)
    fill := int(args[4].AsNumber())
    gocv.Line(&illu, p, q, c, fill)
    return lisp.NewPrimitive(illu), nil
}

// (gocv:rect illu rect color fill)
func gocvRectangle(args []lisp.SExpression) (lisp.SExpression, error) {
    illu := args[0].AsPrimitive().(gocv.Mat)
    rect := args[1].AsPrimitive().(image.Rectangle)
    c := args[2].AsPrimitive().(color.RGBA)
    fill := int(args[3].AsNumber())
    gocv.Rectangle(&illu, rect, c, fill)
    return lisp.NewPrimitive(illu), nil
}

// TODO: cant pick a font yet
// NOTE: text cant be drawn at an angle, so has to be drawn then rotated
// (gocv:text illu text origin scale color fill)
func gocvText(args []lisp.SExpression) (lisp.SExpression, error) {
    illu := args[0].AsPrimitive().(gocv.Mat)
    txt := args[1].AsPrimitive().(string)
    origin := args[2].AsPrimitive().(image.Point)
    scale := args[3].AsNumber()
    c := args[4].AsPrimitive().(color.RGBA)
    fill := int(args[5].AsNumber())
    gocv.PutText(&illu, txt, origin, gocv.FontHersheySimplex, scale, c, fill)
    return lisp.NewPrimitive(illu), nil
}

// (gocv:rotation_matrix2D cx cy degrees scale)
func rotationMatrix(args []lisp.SExpression) (lisp.SExpression, error) {
	x, y := int(args[0].AsNumber()), int(args[1].AsNumber())
    degrees := args[2].AsNumber()
    scale := args[3].AsNumber()
    return lisp.NewPrimitive(gocv.GetRotationMatrix2D(image.Pt(x, y), degrees, scale)), nil
}

// (gocv:warp_affine src dst m sx sy)
func warpAffine(args []lisp.SExpression) (lisp.SExpression, error) {
    src := args[0].AsPrimitive().(gocv.Mat)
    dst := args[1].AsPrimitive().(gocv.Mat)
    m := args[2].AsPrimitive().(gocv.Mat)
	x, y := int(args[3].AsNumber()), int(args[4].AsNumber())
    gocv.WarpAffine(src, &dst, m, image.Pt(x, y))
	return lisp.NewPrimitive(true), nil
}

// (point2D x y) -> image.Point primitive
func newPoint2D(args []lisp.SExpression) (lisp.SExpression, error) {
	x, y := int(args[0].AsNumber()), int(args[1].AsNumber())
	return lisp.NewPrimitive(image.Pt(x, y)), nil
}

// (make-rectangle minx miny maxx maxy) -> image.Rectangle primitive
func newRectangle(args []lisp.SExpression) (lisp.SExpression, error) {
	px, py := int(args[0].AsNumber()), int(args[1].AsNumber())
	qx, qy := int(args[2].AsNumber()), int(args[3].AsNumber())
	r := image.Rectangle{image.Pt(px, py), image.Pt(qx, qy)}
	return lisp.NewPrimitive(r), nil
}

func rectUnion(args []lisp.SExpression) (lisp.SExpression, error) {
    r1 := args[0].AsPrimitive().(image.Rectangle)
    r2 := args[1].AsPrimitive().(image.Rectangle)
    return lisp.NewPrimitive(r1.Union(r2)), nil
}

func sine(args []lisp.SExpression) (lisp.SExpression, error) {
    return lisp.NewPrimitive(math.Sin(args[0].AsNumber())), nil
}

func cosine(args []lisp.SExpression) (lisp.SExpression, error) {
    return lisp.NewPrimitive(math.Cos(args[0].AsNumber())), nil
}

func ignore(args []lisp.SExpression) (lisp.SExpression, error) {
    return lisp.NewPrimitive(true), nil    
}
