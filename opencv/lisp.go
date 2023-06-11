package opencv

import (
	"image"
	"image/color"
	"math"

	"github.com/deosjr/whistle/lisp"
	"gocv.io/x/gocv"
)

var (
	beamerWidth, beamerHeight = 1280, 720
)

// wrapper around gocv for use in lisp code

func Load(env *lisp.Env) {
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
	env.AddBuiltin("sqrt", sqrt)
}

// TODO: defer close? memory leak otherwise?
// dont want to write close in lisp, so for now we'll need some memory mngment
// outside of it (ie keeping track in Go of created mats and closing them)
// NOTE: gocv.Mat is unhashable, so we cant even store it in datalog anyways
var Illus = []gocv.Mat{}

func newIllumination(args []lisp.SExpression) (lisp.SExpression, error) {
	illu := gocv.NewMatWithSize(beamerHeight, beamerWidth, gocv.MatTypeCV8UC3)
	Illus = append(Illus, illu)
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

func sqrt(args []lisp.SExpression) (lisp.SExpression, error) {
	return lisp.NewPrimitive(math.Sqrt(args[0].AsNumber())), nil
}
