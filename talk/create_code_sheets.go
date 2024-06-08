package talk

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"

	"gocv.io/x/gocv"
)

const (
	chessWidth   = 13
	chessHeight  = 7
	blockWidth   = 100
	blockHeight  = 100
	maxAttempts  = 100
	canvasWidth  = 1920
	canvasHeight = 1080
)

var cornerDots = []image.Point{
	image.Pt(0, 0),
	image.Pt(0, -1),
	image.Pt(-1, -1),
	image.Pt(-1, 0),
	image.Pt(chessWidth, 0),
	image.Pt(chessWidth, -1),
	image.Pt(chessWidth-1, -1),
	image.Pt(chessWidth-1, 0),
	image.Pt(0, chessHeight),
	image.Pt(0, chessHeight-1),
	image.Pt(-1, chessHeight-1),
	image.Pt(-1, chessHeight),
	image.Pt(chessWidth, chessHeight),
	image.Pt(chessWidth, chessHeight-1),
	image.Pt(chessWidth-1, chessHeight-1),
	image.Pt(chessWidth-1, chessHeight),
}

func drawChessBoard(img *gocv.Mat) {
	img.SetTo(gocv.NewScalar(255, 255, 255, 0))

	for i := 0; i < chessHeight; i++ {
		for j := 0; j < chessWidth; j++ {
			if (i+j)%2 == 0 {
				rect := image.Rect(j*blockWidth, i*blockHeight, (j+1)*blockWidth, (i+1)*blockHeight)
				gocv.Rectangle(img, rect, color.RGBA{0, 0, 0, 0}, -1)
			}
		}
	}
}

func drawCorners(img *gocv.Mat) {
	for cidx := 0; cidx < len(cornerDots); cidx += 4 {
		pts := make([]image.Point, 4)
		for i := 0; i < 4; i++ {
			px := cornerDots[cidx+i].X
			py := cornerDots[cidx+i].Y
			if px <= 0 {
				px = blockWidth + px*blockWidth
			} else {
				px = px * blockWidth
			}
			if py <= 0 {
				py = blockHeight + py*blockHeight
			} else {
				py = py * blockHeight
			}
			pts[i] = image.Pt(int(px), int(py))
		}
		ptsp := [][]image.Point{pts}
		pointsVector := gocv.NewPointsVectorFromPoints(ptsp)
		defer pointsVector.Close()
		color := cornerColors[1+(cidx/4)]
		gocv.FillPoly(img, pointsVector, color)
	}
}

func rotateImage(img gocv.Mat, angle float64) gocv.Mat {
	// Find the center of the image
	center := image.Pt(img.Cols()/2, img.Rows()/2)

	// Calculate the rotation matrix for the given angle
	matrix := gocv.GetRotationMatrix2D(center, angle, 1.0)

	// Calculate absolute cos and sin of the angle
	absCos := math.Abs(math.Cos(angle * math.Pi / 180))
	absSin := math.Abs(math.Sin(angle * math.Pi / 180))

	// Compute new width and height for the image that can fit the rotated image
	boundW := int(float64(img.Rows())*absSin + float64(img.Cols())*absCos)
	boundH := int(float64(img.Rows())*absCos + float64(img.Cols())*absSin)

	// Adjust the rotation matrix to the new width and height
	matrix.SetDoubleAt(0, 2, matrix.GetDoubleAt(0, 2)+float64(boundW)/2-float64(center.X))
	matrix.SetDoubleAt(1, 2, matrix.GetDoubleAt(1, 2)+float64(boundH)/2-float64(center.Y))

	// Perform the affine transformation (rotation)
	rotated := gocv.NewMat()
	gocv.WarpAffineWithParams(img, &rotated, matrix, image.Pt(boundW, boundH), gocv.InterpolationLinear, gocv.BorderConstant, color.RGBA{255, 255, 255, 0})

	return rotated
}

func scaleImage(img gocv.Mat) gocv.Mat {
	maxDimension := float64(img.Rows())
	if img.Cols() > img.Rows() {
		maxDimension = float64(img.Cols())
	}
	scaleFactor := 480.0 / maxDimension
	scaledWidth := int(float64(img.Cols()) * scaleFactor)
	scaledHeight := int(float64(img.Rows()) * scaleFactor)

	scaled := gocv.NewMat()
	gocv.Resize(img, &scaled, image.Pt(scaledWidth, scaledHeight), 0, 0, gocv.InterpolationLinear)
	return scaled
}

func drawCode(height, width int) gocv.Mat {
	codeImage := gocv.NewMatWithSize(height*blockHeight, width*blockWidth, gocv.MatTypeCV8UC3)
	codeImage.SetTo(gocv.NewScalar(255, 255, 255, 0))

	codePattern := []image.Point{
		{0, 0},
		{0, 1},
		{0, 2},
		{0, width - 4},
		{0, width - 3},
		{0, width - 2},
		{1, 0},
		{1, width - 2},
		{2, 0},
		{2, width - 2},
		{height - 4, 0},
		{height - 4, width - 2},
		{height - 3, 0},
		{height - 3, width - 2},
		{height - 2, 0},
		{height - 2, 1},
		{height - 2, 2},
		{height - 2, width - 4},
		{height - 2, width - 3},
		{height - 2, width - 2},
	}

	for _, p := range codePattern {
		randomColorIndex := rand.Intn(len(cornerColors) - 1)
		center := image.Pt(p.Y*blockWidth+blockWidth/2, p.X*blockHeight+blockHeight/2)
		gocv.Circle(&codeImage, center, 47, cornerColors[randomColorIndex+1], -1)
	}

	return codeImage
}

func placeCalibrationPattern(canvas *gocv.Mat) {
	width := canvas.Cols()
	height := canvas.Rows()
	for r := 0; r < 2; r++ {
		for c := 0; c < 2; c++ {
			color := cornerColors[1+(r*2+c)]
			center := image.Pt((width/2)+((c-1)*blockWidth)+blockWidth/2, (height/2)+((r-1)*blockHeight+blockWidth/2))
			gocv.CircleWithParams(canvas, center, (blockWidth / 3), color, -1, gocv.LineAA, 0)
		}
	}
}

func canPlace(sheetImage gocv.Mat, canvas gocv.Mat, x, y int) bool {
	if x+sheetImage.Cols() > canvas.Cols() || y+sheetImage.Rows() > canvas.Rows() {
		return false
	}
	submat := canvas.Region(image.Rect(x, y, x+sheetImage.Cols(), y+sheetImage.Rows()))
	defer submat.Close()

	white := gocv.NewMatWithSize(submat.Rows(), submat.Cols(), gocv.MatTypeCV8UC3)
	white.SetTo(gocv.NewScalar(255, 255, 255, 0))
	defer white.Close()

	diff := gocv.NewMat()
	defer diff.Close()
	gocv.Compare(submat, white, &diff, gocv.CompareNE)
	gocv.CvtColor(diff, &diff, gocv.ColorBGRToGray)
	return gocv.CountNonZero(diff) == 0
}

func placeCodes(canvas *gocv.Mat) {
	codeSheets := []gocv.Mat{}
	for i := 0; i < 4; i++ {
		codeSheet := drawCode(10, 15)
		defer codeSheet.Close()
		codeSheets = append(codeSheets, rotateImage(codeSheet, rand.Float64()*360))
	}

	for _, codeSheet := range codeSheets {
		scaledCodeSheet := scaleImage(codeSheet)
		defer scaledCodeSheet.Close()

		for attempt := 0; attempt < maxAttempts; attempt++ {
			if canvas.Cols() <= scaledCodeSheet.Cols() || canvas.Rows() <= scaledCodeSheet.Rows() {
				fmt.Println("Code sheet is too large to fit on the canvas.")
				continue
			}
			x := rand.Intn(canvas.Cols() - scaledCodeSheet.Cols())
			y := rand.Intn(canvas.Rows() - scaledCodeSheet.Rows())
			if canPlace(scaledCodeSheet, *canvas, x, y) {
				region := canvas.Region(image.Rect(x, y, x+scaledCodeSheet.Cols(), y+scaledCodeSheet.Rows()))
				defer region.Close()
				scaledCodeSheet.CopyToWithMask(&region, scaledCodeSheet)
				break
			}
		}
		codeSheet.Close()
	}
}

func getSheet(key int, img *gocv.Mat) {
	switch key {
	case 'z':
		*img = gocv.NewMatWithSize(chessHeight*blockHeight, chessWidth*blockWidth, gocv.MatTypeCV8UC3)
		img.SetTo(gocv.NewScalar(255, 255, 255, 0))
	case 'x':
		*img = gocv.NewMatWithSize(chessHeight*blockHeight, chessWidth*blockWidth, gocv.MatTypeCV8UC3)
		img.SetTo(gocv.NewScalar(255, 255, 255, 0))
		drawChessBoard(img)
		drawCorners(img)
	case 'c':
		*img = gocv.NewMatWithSize(chessHeight*blockHeight, chessWidth*blockWidth, gocv.MatTypeCV8UC3)
		img.SetTo(gocv.NewScalar(255, 255, 255, 0))
		drawCorners(img)
		placeCalibrationPattern(img)
	case 'v':
		*img = gocv.NewMatWithSize(chessHeight*blockHeight, chessWidth*blockWidth, gocv.MatTypeCV8UC3)
		img.SetTo(gocv.NewScalar(255, 255, 255, 0))
		drawCorners(img)
	case 'b':
		*img = gocv.NewMatWithSize(chessHeight*blockHeight, chessWidth*blockWidth, gocv.MatTypeCV8UC3)
		img.SetTo(gocv.NewScalar(255, 255, 255, 0))
		drawChessBoard(img)
	case 'a':
		*img = gocv.NewMatWithSize(canvasHeight, canvasWidth, gocv.MatTypeCV8UC3)
		img.SetTo(gocv.NewScalar(255, 255, 255, 0))
		placeCodes(img)
	}
}

func createSheets() {
	window := gocv.NewWindow("projection")
	img := gocv.NewMatWithSize(chessHeight*blockHeight, chessWidth*blockWidth, gocv.MatTypeCV8UC3)
	defer img.Close()
	drawChessBoard(&img)
	drawCorners(&img)

	for {
		key := window.WaitKey(1)
		getSheet(key, &img)
		window.IMShow(img)
	}
}
