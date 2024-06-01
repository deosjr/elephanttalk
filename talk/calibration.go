package talk

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"log"
	"sync"
	"time"

	"gocv.io/x/gocv"
)

type calibrationResults struct {
	pixelsPerCM     float64
	displacement    point
	displayRatio    float64
	referenceColors []color.RGBA
	rvec            gocv.Mat
	tvec            gocv.Mat
	mtx             gocv.Mat
	dist            gocv.Mat
}

type straightChessboard struct {
	mapx gocv.Mat
	mapy gocv.Mat
	roi  image.Rectangle
	M    gocv.Mat
}

type RectanglePoint struct {
	Rect   image.Rectangle
	Center image.Point
}

// Curiously enough the Go code does not work with histograms of 32x32x32 but only with histograms > 128x128x128
const W = 12                     // nb checkers on board horizontal
const H = 6                      // nb checkers on board vertical
const HIST_SIZE = 160            // color histogram bins per dimension
const THETA = 0.25               // probability threshold for color prediction
const MIN_NB_CHKBRD_FOUND = 50   // minimum number of frames with checkerboard found
const MIN_NB_COLOR_SAMPLES = 100 // minimum number of color samples for color models

const NB_CLRD_CHCKRS = 4              // number of colored checkers
const CW = 100                        // checker projected resolution (W) (pixels per checker, if you measure the checker size in mm, it can be that)
const CH = 100                        // checker projected resolution (H) (pixels per checker, if you measure the checker size in mm, it can be that)
const CB = 5                          // checker projected resolution boundary
const EPSILON = 2.220446049250313e-16 // epsilon for division by zero prevention
const WAIT = 5                        // wait time in milliseconds (don't make it too large)

var colorBlack = color.RGBA{0, 0, 0, 255}
var colorWhite = color.RGBA{255, 255, 255, 255}
var colorRed = color.RGBA{255, 0, 0, 255}
var colorGreen = color.RGBA{0, 255, 0, 255}
var colorBlue = color.RGBA{0, 0, 255, 255}
var colorYellow = color.RGBA{255, 255, 0, 255}
var colorCyan = color.RGBA{0, 255, 255, 255}
var colorMagenta = color.RGBA{255, 0, 255, 255}

var cornerColors = []color.RGBA{
	{245, 34, 45, 255},           // red
	{56, 158, 13, 255},           // green
	{57 * 2, 16 * 2.5, 133, 255}, // purple
	{250, 140 * 1.5, 22, 255},    // orange
}

var masksGlob = []*Deque{}

// Sample the colors (create Histogram) of the colored checkers using a mask for each colored checker
func sampleColorWithMask(frame gocv.Mat, masks [4]gocv.Mat, colorHistSums []gocv.Mat, nonColorHistSums []gocv.Mat, cidx int) {
	frame_clrsp := gocv.NewMat()
	defer frame_clrsp.Close()
	// frame_clrsp = frame.Clone()
	gocv.CvtColor(frame, &frame_clrsp, gocv.ColorBGRToLuv)

	// Invert the mask
	maskInv := gocv.NewMat()
	defer maskInv.Close()
	gocv.BitwiseNot(masks[cidx], &maskInv)

	colorHist := gocv.NewMat()
	gocv.CalcHist([]gocv.Mat{frame_clrsp}, []int{0, 1, 2}, masks[cidx], &colorHist,
		[]int{HIST_SIZE, HIST_SIZE, HIST_SIZE}, []float64{0, 256, 0, 256, 0, 256}, false)

	nonColorHist := gocv.NewMat()
	gocv.CalcHist([]gocv.Mat{frame_clrsp}, []int{0, 1, 2}, maskInv, &nonColorHist,
		[]int{HIST_SIZE, HIST_SIZE, HIST_SIZE}, []float64{0, 256, 0, 256, 0, 256}, false)

	if colorHistSums[cidx].Empty() {
		colorHistSums[cidx].Close()
		colorHistSums[cidx] = colorHist.Clone()

	} else {
		gocv.Add(colorHistSums[cidx], colorHist, &colorHistSums[cidx])
	}
	colorHist.Close()

	if nonColorHistSums[cidx].Empty() {
		nonColorHistSums[cidx].Close()
		nonColorHistSums[cidx] = nonColorHist.Clone()

	} else {
		gocv.Add(nonColorHistSums[cidx], nonColorHist, &nonColorHistSums[cidx])
	}
	nonColorHist.Close()
}

// Sample the colors of the colored checkers using the four corner points
// This should be simplified by calling sampleColorWithMask
func sampleColors(frame gocv.Mat, cornerPointsProj [][]interface{},
	colorHistSums []gocv.Mat, nonColorHistSums []gocv.Mat, cornersWindow *gocv.Window) {

	frame_clrsp := gocv.NewMat()
	defer frame_clrsp.Close()
	// frame_clrsp = frame.Clone()
	gocv.CvtColor(frame, &frame_clrsp, gocv.ColorBGRToLuv)

	finalCrops := gocv.NewMat()

	// Loop over corner_points and convert points to gocv.PointsVector
	for cidx, cornerPoint := range cornerPointsProj {
		pointsProj := cornerPoint[0].([][]int)

		// Create a gocv.PointsVector from points
		pointsVector := gocv.NewPointsVector()
		defer pointsVector.Close()

		// Convert points to gocv.Point
		imagePoints := gocv.NewPointVector()
		defer imagePoints.Close()
		projPointsArr := []gocv.Point2f{}
		for _, pt := range pointsProj {
			imagePoint := image.Pt(pt[0], pt[1])
			imagePoints.Append(imagePoint)

			point2f := gocv.Point2f{X: float32(imagePoint.X), Y: float32(imagePoint.Y)}
			projPointsArr = append(projPointsArr, point2f)
		}
		pointsVector.Append(imagePoints)

		// Create a mask from the polygon to extract the color histogram from the frame for the given corner
		mask := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8U)
		defer mask.Close()
		gocv.FillPoly(&mask, pointsVector, colorWhite)

		// Invert the mask
		maskInv := gocv.NewMat()
		defer maskInv.Close()
		gocv.BitwiseNot(mask, &maskInv)

		colorHist := gocv.NewMat()
		gocv.CalcHist([]gocv.Mat{frame_clrsp}, []int{0, 1, 2}, mask, &colorHist,
			[]int{HIST_SIZE, HIST_SIZE, HIST_SIZE}, []float64{0, 256, 0, 256, 0, 256}, false)

		nonColorHist := gocv.NewMat()
		gocv.CalcHist([]gocv.Mat{frame_clrsp}, []int{0, 1, 2}, maskInv, &nonColorHist,
			[]int{HIST_SIZE, HIST_SIZE, HIST_SIZE}, []float64{0, 256, 0, 256, 0, 256}, false)

		if colorHistSums[cidx].Empty() {
			colorHistSums[cidx].Close()
			colorHistSums[cidx] = colorHist.Clone()

		} else {
			gocv.Add(colorHistSums[cidx], colorHist, &colorHistSums[cidx])
		}
		colorHist.Close()

		if nonColorHistSums[cidx].Empty() {
			nonColorHistSums[cidx].Close()
			nonColorHistSums[cidx] = nonColorHist.Clone()

		} else {
			gocv.Add(nonColorHistSums[cidx], nonColorHist, &nonColorHistSums[cidx])
		}
		nonColorHist.Close()

		if len(projPointsArr) == 4 {
			// Extract the poly from the image using the mask
			poly := gocv.NewMat()
			defer poly.Close()
			gocv.BitwiseAndWithMask(frame, frame, &poly, mask)

			cornerDots := [][]int{{0, 0}, {CW, 0}, {CW, CH}, {0, CH}}
			p2sPlane := []gocv.Point2f{}
			for i := 0; i < len(cornerDots); i++ {
				point := image.Pt(cornerDots[i][0], cornerDots[i][1])
				p2sPlane = append(p2sPlane, gocv.Point2f{X: float32(point.X), Y: float32(point.Y)})
			}

			projPtsV := gocv.NewPoint2fVectorFromPoints(projPointsArr)
			defer projPtsV.Close()
			planePtsV := gocv.NewPoint2fVectorFromPoints(p2sPlane)
			defer planePtsV.Close()

			M := gocv.GetPerspectiveTransform2f(projPtsV, planePtsV)
			defer M.Close()
			tPoly := gocv.NewMat()
			defer tPoly.Close()
			gocv.WarpPerspective(poly, &tPoly, M, image.Pt(CW, CH))

			// Create the rectangle for the region of interest
			rect := image.Rect(CB, CB, CW-CB, CH-CB)
			// Crop the transformed_polygon
			finalCrop := tPoly.Region(rect)
			defer finalCrop.Close()

			if finalCrops.Empty() {
				finalCrops.Close()
				finalCrops = finalCrop.Clone()

			} else {
				gocv.Vconcat(finalCrops, finalCrop, &finalCrops)
			}
		}
	}

	cornersWindow.IMShow(finalCrops)
	finalCrops.Close()
}

// Project the corner points to the image plane
func projectCornerPoints(cornerDotVector gocv.Point3fVector, mtx, dist, rvec, tvec gocv.Mat) [][]interface{} {
	projPoints := gocv.NewPoint2fVector()
	defer projPoints.Close()
	jacobian := gocv.NewMat()
	defer jacobian.Close()
	gocv.ProjectPoints(cornerDotVector, rvec, tvec, mtx, dist, projPoints, &jacobian, 0)

	// cornerPointsProj will store the results
	var cornerPointsProj []([]interface{})

	for pidx := 0; pidx < projPoints.Size()-3; pidx += 4 {
		proj_pt1 := []int{int(projPoints.At(pidx).X), int(projPoints.At(pidx).Y)}
		proj_pt2 := []int{int(projPoints.At(pidx + 1).X), int(projPoints.At(pidx + 1).Y)}
		proj_pt3 := []int{int(projPoints.At(pidx + 2).X), int(projPoints.At(pidx + 2).Y)}
		proj_pt4 := []int{int(projPoints.At(pidx + 3).X), int(projPoints.At(pidx + 3).Y)}

		cornerPointsProj = append(cornerPointsProj, []interface{}{
			[]([]int){proj_pt1, proj_pt2, proj_pt3, proj_pt4},
			cornerColors[pidx/4],
		})
	}

	return cornerPointsProj
}

// Uses Bayes' Theorem to calculate the probability of a pixel belonging to a colored checker
func calcBayesColorModel(cSumHist, ncSumHist gocv.Mat, bayesColorModel *gocv.Mat) (float64, error) {
	if cSumHist.Empty() || ncSumHist.Empty() {
		log.Fatal("colorHistSums is empty")
		return 0.0, fmt.Errorf("invalid mat")
	}
	// fmt.Println("cSumHist", cSumHist.Size())
	// Print3DMatValues32f(cSumHist)
	// fmt.Println("ncSumHist", ncSumHist.Size())
	// Print3DMatValues32f(ncSumHist)

	cHistSumSc, _ := sumMat(cSumHist)   // Total sum of the colors in the colored checkers per checker
	ncHistSumSc, _ := sumMat(ncSumHist) // Total sum of the colors in the rest of the image per checker (in the not space of the colored checkers)

	// fmt.Println("cHistSumSc", cHistSumSc, "ncHistSumSc", ncHistSumSc)

	// Hit it Bayes!
	pColor := cHistSumSc / (cHistSumSc + ncHistSumSc) // P(checker_color)  : Chance of hitting the given colored checker, given a random color
	pNonColor := 1 - pColor                           // P(~checker_color) : Chance of not hitting the given colored checker, given a random color

	// fmt.Println("pColor", pColor, "pNonColor", pNonColor)

	cHistSumScMat := gocv.NewMatFromScalar(gocv.NewScalar(cHistSumSc, 0, 0, 0), gocv.MatTypeCV64F)
	defer cHistSumScMat.Close()
	pRgbColor := gocv.NewMat()
	defer pRgbColor.Close()
	gocv.Divide(cSumHist, cHistSumScMat, &pRgbColor) // P(rgb|checker_color)  : Chance distribution in color space of belonging to colored checker

	// fmt.Print("pRgbColor: ")
	// Print3DMatValues32f(pRgbColor)
	// pRgbColor, _ := divideMatByScalar(cSumHist, cHistSumSc)      // Chance distribution in color space of belonging to colored checker

	ncHistSumScMat := gocv.NewMatFromScalar(gocv.NewScalar(ncHistSumSc, 0, 0, 0), gocv.MatTypeCV64F)
	defer ncHistSumScMat.Close()
	pRgbNonColor := gocv.NewMat()
	defer pRgbNonColor.Close()
	gocv.Divide(ncSumHist, ncHistSumScMat, &pRgbNonColor) // P(rgb|~checker_color) : Chance distribution in color space of not belonging to colored checker

	// fmt.Print("pRgbNonColor: ")
	// Print3DMatValues32f(pRgbNonColor)
	// pRgbNonColor, _ := divideMatByScalar(ncSumHist, ncHistSumSc) // Chance distribution in color space of not belonging to colored checker

	pColorMat := gocv.NewMatFromScalar(gocv.NewScalar(pColor, 0, 0, 0), gocv.MatTypeCV64F)
	defer pColorMat.Close()

	// pNonColorMat := gocv.NewMatFromScalar(gocv.NewScalar(pNonColor, 0, 0, 0), gocv.MatTypeCV64F)
	// defer pNonColorMat.Close()

	pRgbColorMul := gocv.NewMat()
	defer pRgbColorMul.Close()
	gocv.Multiply(pRgbColor, pColorMat, &pRgbColorMul)

	// minVal, maxVal, _, _ := gocv.MinMaxIdx(pRgbColorMul)
	// fmt.Println("MinVal", minVal, "MaxVal", maxVal)
	// sumPRgbColorMul, _ := sumMat(pRgbColorMul)
	// fmt.Println("pRgbColorMul", pRgbColorMul.Size(), "sumPRgbColorMul", sumPRgbColorMul)

	// pRgbNonColorMul := gocv.NewMat()
	// defer pRgbNonColorMul.Close()
	// gocv.Multiply(pRgbNonColor, pNonColorMat, &pRgbNonColorMul)
	// minValpRgbNonColorMul, maxValpRgbNonColorMul, _, _ := gocv.MinMaxIdx(pRgbNonColorMul)
	// fmt.Println("MinVal", minValpRgbNonColorMul, "MaxVal", maxValpRgbNonColorMul)
	// sumpRgbNonColorMul, _ := sumMat(pRgbNonColorMul)
	// fmt.Println("pRgbNonColorMul", pRgbNonColorMul.Size(), "sumpRgbNonColorMul", sumpRgbNonColorMul)

	pRGB := gocv.NewMat()
	defer pRGB.Close()

	// gocv.Add(pRgbColorMul, pRgbNonColorMul, &pRGB)
	gocv.AddWeighted(pRgbColor, pColor, pRgbNonColor, pNonColor, 0, &pRGB) // P(rgb) : Sum and weigh chance distribution color/non-color
	pRGB, _ = ensureNonZero(pRGB)                                          // Make sure we don't divide by zero

	// minValpRGB, maxValpRGB, _, _ := gocv.MinMaxIdx(pRGB)
	// fmt.Println("MinVal", minValpRGB, "MaxVal", maxValpRGB)
	// sumPRGB, _ := sumMat(pRGB)
	// fmt.Println("Size", pRGB.Size(), "pRGB", sumPRGB)
	// Print3DMatValues32f(pRGB)

	// minValpRgbColorMul, maxValpRgbColorMul, _, _ := gocv.MinMaxIdx(pRgbColorMul)
	// fmt.Println("MinVal", minValpRgbColorMul, "MaxVal", maxValpRgbColorMul)
	// sumPRgbColorMul, _ = sumMat(pRgbColorMul)
	// fmt.Println("pRgbColorMul", pRgbColorMul.Size(), "sumPRgbColorMul", sumPRgbColorMul)
	// Print3DMatValues32f(pRgbColorMul)

	gocv.Divide(pRgbColorMul, pRGB, bayesColorModel) // P(checker_color|rgb) : Bayes' theorem
	// gocv.Normalize(*pRGBColorChance, pRGBColorChance, 0, 1, gocv.NormMinMax)
	// minValpRGBColorChance, maxValpRGBColorChance, _, _ := gocv.MinMaxIdx(*bayesColorModel)
	// sumPRGBColorChance, _ := sumMat(*bayesColorModel)
	// fmt.Println("MinVal", minValpRGBColorChance, "MaxVal", maxValpRGBColorChance, "Sum", sumPRGBColorChance)
	// Print3DMatValues32f(*pRGBColorChance)

	// Calculate the scaling factor for mapping the frame RGB color dimensions (256x256x256) to
	// histogram color space dimensions (eg.: 32x32x32)
	// Given that we have 256 colors in three channels, we map each of the three dimensions to the 32x32x32 color space
	// So the probability of a pixel belonging to the checker color comes from looking in pRGBColorChance at the
	// given RGB color's location in the 32x32x32 3D chance distribution
	colorSpaceFactor := 1.0 / 256.0 * float64(bayesColorModel.Size()[0])
	return colorSpaceFactor, nil
}

// Predict the occurrence of a colored checkers color in the given frame, takes a colorModel returns a mask
func predictCheckerColor(frame gocv.Mat, colorModel gocv.Mat, colorSpaceFactor float64) gocv.Mat {
	frame_clrsp := gocv.NewMat()
	defer frame_clrsp.Close()
	// Unsure if Luv is the best color space for this, may consider CrCb or HSV
	gocv.CvtColor(frame, &frame_clrsp, gocv.ColorBGRToLuv) // Convert the frame to Luv color space

	frameFloat := gocv.NewMat()
	defer frameFloat.Close()
	frame_clrsp.ConvertTo(&frameFloat, gocv.MatTypeCV32FC3)

	csFactorMat := gocv.NewMatFromScalar(gocv.NewScalar(colorSpaceFactor, colorSpaceFactor, colorSpaceFactor, colorSpaceFactor), gocv.MatTypeCV64F)
	defer csFactorMat.Close()
	csIndices := gocv.NewMat()
	defer csIndices.Close()

	// frameFloat has all RGB colors (256x256x256), scale and save each color as an index in histogram (32x32x32)
	// color space with dimensions of the webcam frame into a pixel->colorspace lookup table (csIndices)
	gocv.Multiply(frameFloat, csFactorMat, &csIndices)
	csIndices.ConvertTo(&csIndices, gocv.MatTypeCV16SC3)

	finalMask := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8U)
	localMasks := make(chan gocv.Mat, csIndices.Rows())

	// The next bit is still a bit slow :( not sure how to optimize it further

	// Get the whole data of csIndices at once
	allData := csIndices.ToBytes()
	step := csIndices.Cols() * 6 // 2 bytes per short * 3 channels

	// Use a worker pool to limit the number of concurrent goroutines
	var wg sync.WaitGroup
	numWorkers := 10 // Adjust based on your CPU or concurrency needs
	rowCh := make(chan int, csIndices.Rows())

	// Worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for y := range rowCh {
				localMask := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8U)

				// Access row data from pre-fetched allData
				rowStart := y * step

				for x := 0; x < csIndices.Cols(); x++ {
					offset := rowStart + x*6
					ib := binary.LittleEndian.Uint16(allData[offset : offset+2])
					ig := binary.LittleEndian.Uint16(allData[offset+2 : offset+4])
					ir := binary.LittleEndian.Uint16(allData[offset+4 : offset+6])

					if float64(colorModel.GetFloatAt3(int(ib), int(ig), int(ir))) > THETA {
						localMask.SetUCharAt(y, x, 255)
					}
				}

				localMasks <- localMask.Clone()
				localMask.Close()
			}
		}()
	}

	// Distribute rows to workers
	go func() {
		for y := 0; y < csIndices.Rows(); y++ {
			rowCh <- y
		}
		close(rowCh)
	}()

	// Close the localMasks channel when all workers are done
	go func() {
		wg.Wait()
		close(localMasks)
	}()

	for mask := range localMasks {
		gocv.BitwiseOr(finalMask, mask, &finalMask)
		mask.Close()
	}

	// Erode and dilate the mask to remove noise
	gocv.MorphologyEx(finalMask, &finalMask, gocv.MorphErode, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(3, 3)))
	gocv.MorphologyEx(finalMask, &finalMask, gocv.MorphDilate, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(5, 5)))

	return finalMask
}

// Predict the colored checkers in the frame and return the number of found checkers and their rectangle locations
func predictCheckerColors(frame gocv.Mat, canvas *gocv.Mat,
	cornerColors []color.RGBA, colorModels []gocv.Mat, colorSpaceFactor float64) (int, [4]image.Rectangle) {

	nbFoundRects := 0
	checkerRectangles := [4]image.Rectangle{}

	for pidx, colorModel := range colorModels {
		mask := predictCheckerColor(frame, colorModel, colorSpaceFactor)

		maskClone := mask.Clone()
		masksGlob[pidx].Push(&maskClone)
		mask.Close()

		frameColor := gocv.NewMatWithSize(canvas.Rows(), canvas.Cols(), gocv.MatTypeCV8UC3)
		defer frameColor.Close()

		if masksGlob[pidx].Size() >= 5 {
			masksSum := gocv.NewMatWithSize(canvas.Rows(), canvas.Cols(), gocv.MatTypeCV8U)
			defer masksSum.Close()
			for maskPast := range masksGlob[pidx].Iter() {
				gocv.Add(*maskPast, masksSum, &masksSum)
			}

			color := gocv.NewScalar(
				float64(cornerColors[pidx].B),
				float64(cornerColors[pidx].G),
				float64(cornerColors[pidx].R),
				255,
			)
			frameColor.SetTo(color)
			frameColor.CopyToWithMask(canvas, masksSum)

			checkerHullPointsVector := gocv.NewPointsVector()
			defer checkerHullPointsVector.Close()
			rectanglePoints := []RectanglePoint{}
			contoursVector := gocv.FindContours(masksSum, gocv.RetrievalList, gocv.ChainApproxSimple)
			defer contoursVector.Close()
			for _, contour := range contoursVector.ToPoints() {
				contourPoints := gocv.NewPointVectorFromPoints(contour)
				defer contourPoints.Close()
				hullIndices := gocv.NewMat()
				defer hullIndices.Close()
				// Create convex hull from contour (closes gaps in the contour)
				gocv.ConvexHull(contourPoints, &hullIndices, false, true)

				// Convert indices to actual points
				hullPoints := make([]image.Point, hullIndices.Rows())
				for j := 0; j < hullIndices.Rows(); j++ {
					x := int(hullIndices.GetIntAt(j, 0))
					y := int(hullIndices.GetIntAt(j, 1))
					hullPoints[j] = image.Point{x, y}
				}

				// Add this hull's PointsVector to the main PointsVector
				hullPointsVector := gocv.NewPointsVector()
				defer hullPointsVector.Close()
				hullPointVector := gocv.NewPointVectorFromPoints(hullPoints)
				defer hullPointVector.Close()
				hullPointsVector.Append(hullPointVector)

				contourMask := gocv.NewMatWithSize(canvas.Rows(), canvas.Cols(), gocv.MatTypeCV8U)
				defer contourMask.Close()
				gocv.FillPoly(&contourMask, hullPointsVector, colorWhite)
				area := getMaskArea(contourMask)
				area_pct := (float64(area) / float64(canvas.Cols()*canvas.Rows())) * 100

				if area_pct > 0.25 { // Hard coded checker area threshold :(
					rectanglePoints = append(rectanglePoints, RectanglePoint{
						Rect:   gocv.BoundingRect(contourPoints),
						Center: getCenter(contour),
					})

					checkerHullPointsVector.Append(hullPointVector)

				}
				// else {
				// 	hullPointVector.Close()
				// }
			}

			if len(rectanglePoints) > 0 {
				extremePoint := getExtremePoint(rectanglePoints, pidx)
				gocv.RectangleWithParams(canvas, extremePoint.Rect, colorGreen, 1, gocv.LineAA, 0)
				checkerRectangles[pidx] = extremePoint.Rect
				nbFoundRects++
			}
			for hidx := 0; hidx < checkerHullPointsVector.Size(); hidx++ {
				gocv.DrawContours(canvas, checkerHullPointsVector, hidx, colorYellow, 2)
			}
		}
	}

	return nbFoundRects, checkerRectangles
}

// Calibrate the sheet by sampling the colors of the colored checkers from the sheet and updating
// the color models accordingly
func calibrateSheet(frame gocv.Mat, canvas *gocv.Mat, quadrants [4][4]image.Point,
	colorHistSums, nonColorHistSums, colorModels []gocv.Mat, nbHistsSampled *int,
	quadrantWindows [4]*gocv.Window, trackbarsS *gocv.Trackbar, trackbarsV *gocv.Trackbar) (bool, bool) {

	isAllCircleMasksSeen := false
	isSheetCalibrated := false

	circleMasks := [4]gocv.Mat{}
	nbCircleMasks := 0
	for qidx, quadrant := range quadrants {
		cw := int(lineLength(quadrant[0], quadrant[1]))
		ch := int(lineLength(quadrant[1], quadrant[2]))

		pointsVector := gocv.NewPointsVector()
		defer pointsVector.Close()
		imgCornersPtsArr := []gocv.Point2f{}
		imagePoints := gocv.NewPointVector()
		defer imagePoints.Close()
		for _, imagePoint := range quadrant {
			point2f := gocv.Point2f{X: float32(imagePoint.X), Y: float32(imagePoint.Y)}
			imgCornersPtsArr = append(imgCornersPtsArr, point2f)
			imagePoints.Append(imagePoint)
		}
		pointsVector.Append(imagePoints)

		mask := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8U)
		defer mask.Close()
		gocv.FillPoly(&mask, pointsVector, colorWhite)

		poly := gocv.NewMat()
		defer poly.Close()
		gocv.BitwiseAndWithMask(frame, frame, &poly, mask)

		// Project the quadrant in the frame to a rectangular plane
		planeP2f := []gocv.Point2f{{X: 0, Y: 0}, {X: float32(cw), Y: 0}, {X: float32(cw), Y: float32(ch)}, {X: 0, Y: float32(ch)}}

		imgCorners := gocv.NewPoint2fVectorFromPoints(imgCornersPtsArr)
		defer imgCorners.Close()
		straightCorners := gocv.NewPoint2fVectorFromPoints(planeP2f)
		defer straightCorners.Close()

		M := gocv.GetPerspectiveTransform2f(imgCorners, straightCorners)
		defer M.Close()

		tPoly := gocv.NewMat()
		defer tPoly.Close()
		gocv.WarpPerspective(poly, &tPoly, M, image.Pt(cw, ch))

		// Convert BGR to HSV
		hsvImg := gocv.NewMat()
		defer hsvImg.Close()
		gocv.CvtColor(tPoly, &hsvImg, gocv.ColorBGRToHSV)

		// Read the adjustable values
		saturation := float64(trackbarsS.GetPos())
		value := float64(trackbarsV.GetPos())

		// Some filtering to get rid of the black/white checkers
		lbMask0 := gocv.NewScalar(0, saturation, value, 0)
		ubMask0 := gocv.NewScalar(180, 255, 255, 0)

		maskChecker := gocv.NewMat()
		defer maskChecker.Close()
		gocv.InRangeWithScalar(hsvImg, lbMask0, ubMask0, &maskChecker)

		// Erode and dilate the mask to remove noise
		gocv.MorphologyEx(maskChecker, &maskChecker, gocv.MorphErode, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(3, 3)))
		gocv.MorphologyEx(maskChecker, &maskChecker, gocv.MorphDilate, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(5, 5)))

		// Apply the mask to the original image, so we are left with the colored checkers and the dots
		resultImg := gocv.NewMat()
		defer resultImg.Close()
		tPoly.CopyToWithMask(&resultImg, maskChecker)

		colorHists := []gocv.Mat{}
		contours := gocv.FindContours(maskChecker, gocv.RetrievalTree, gocv.ChainApproxSimple)
		defer contours.Close()
		contoursVector := gocv.NewPointsVector()
		defer contoursVector.Close()
		// TODO: create a hull from the contour and use the hull to create a mask
		for _, contour := range contours.ToPoints() {
			contoursDotVector := gocv.NewPointsVector()
			defer contoursDotVector.Close()
			contourVector := gocv.NewPointVectorFromPoints(contour)
			defer contourVector.Close()
			contoursDotVector.Append(contourVector)

			maskDot := gocv.NewMatWithSize(tPoly.Rows(), tPoly.Cols(), gocv.MatTypeCV8U)
			defer maskDot.Close()
			gocv.FillPoly(&maskDot, contoursDotVector, colorWhite)

			if getContourArea(contourVector) > 100 {
				colorHist := gocv.NewMat()
				gocv.CalcHist([]gocv.Mat{tPoly}, []int{0, 1, 2}, maskDot, &colorHist, []int{32, 32, 32}, []float64{0, 256, 0, 256, 0, 256}, false)
				colorHists = append(colorHists, colorHist)
				contoursVector.Append(contourVector)
			}
		}

		// A colored dot is matched with a checker with the 'same' color, by matching the max correllating color histograms
		if len(colorHists) > 1 {
			maxCCorr := float32(-100.0)
			cidx00 := 0
			cidx01 := 0
			for cidx0 := 0; cidx0 < len(colorHists); cidx0++ {
				for cidx1 := cidx0 + 1; cidx1 < len(colorHists); cidx1++ {
					colorHist0 := colorHists[cidx0]
					colorHist1 := colorHists[cidx1]

					corr := gocv.CompareHist(colorHist0, colorHist1, gocv.HistCmpCorrel)
					if corr > maxCCorr {
						maxCCorr = corr
						cidx00 = cidx0
						cidx01 = cidx1
					}
				}
			}

			for _, colorHist := range colorHists {
				colorHist.Close()
			}

			contour_cidx00 := contoursVector.At(cidx00).ToPoints()
			contour_cidx01 := contoursVector.At(cidx01).ToPoints()
			c_cidx00 := getCenter(contour_cidx00)
			c_cidx01 := getCenter(contour_cidx01)

			checker := -1
			checker_c := image.Point{}
			circle := -1
			circle_c := image.Point{}
			if qidx == 0 { // In quadrant 1, the colored checker is to the left and above the dot
				if c_cidx00.X < c_cidx01.X && c_cidx00.Y < c_cidx01.Y {
					checker = cidx00
					checker_c = c_cidx00
					circle = cidx01
					circle_c = c_cidx01
				} else {
					checker = cidx01
					checker_c = c_cidx01
					circle = cidx00
					circle_c = c_cidx00
				}
			} else if qidx == 1 { // In quadrant 2, the colored checker is to the right and above the dot
				if c_cidx00.X > c_cidx01.X && c_cidx00.Y < c_cidx01.Y {
					checker = cidx00
					checker_c = c_cidx00
					circle = cidx01
					circle_c = c_cidx01
				} else {
					checker = cidx01
					checker_c = c_cidx01
					circle = cidx00
					circle_c = c_cidx00
				}
			} else if qidx == 2 { // In quadrant 3, the colored checker is to the left and below the dot
				if c_cidx00.X < c_cidx01.X && c_cidx00.Y > c_cidx01.Y {
					checker = cidx00
					checker_c = c_cidx00
					circle = cidx01
					circle_c = c_cidx01
				} else {
					checker = cidx01
					checker_c = c_cidx01
					circle = cidx00
					circle_c = c_cidx00
				}
			} else if qidx == 3 { // In quadrant 4, the colored checker is to the right and below the dot
				if c_cidx00.X > c_cidx01.X && c_cidx00.Y > c_cidx01.Y {
					checker = cidx00
					checker_c = c_cidx00
					circle = cidx01
					circle_c = c_cidx01
				} else {
					checker = cidx01
					checker_c = c_cidx01
					circle = cidx00
					circle_c = c_cidx00
				}
			}

			gocv.DrawContours(&resultImg, contoursVector, checker, colorWhite, 1)
			gocv.CircleWithParams(&resultImg, checker_c, 2, colorWhite, -1, gocv.LineAA, 0)
			gocv.DrawContours(&resultImg, contoursVector, circle, colorWhite, 1)
			gocv.CircleWithParams(&resultImg, circle_c, 2, colorBlack, -1, gocv.LineAA, 0)

			circleMaskFrame := gocv.NewMatWithSize(tPoly.Rows(), tPoly.Cols(), gocv.MatTypeCV8U)
			defer circleMaskFrame.Close()
			circlePointVector := contoursVector.At(circle)
			circlePointsVector := gocv.NewPointsVector()
			defer circlePointsVector.Close()
			circlePointsVector.Append(circlePointVector)
			gocv.FillPoly(&circleMaskFrame, circlePointsVector, colorWhite)

			// Backproject the circle mask in the straightened quadrant to the original frame
			circleMaskFrameW := gocv.NewMat()
			gocv.WarpPerspectiveWithParams(circleMaskFrame, &circleMaskFrameW, M,
				image.Pt(frame.Cols(), frame.Rows()), gocv.InterpolationLinear|gocv.WarpInverseMap, gocv.BorderConstant, colorBlack)

			circleMasks[qidx] = circleMaskFrameW
			nbCircleMasks++
		}

		quadrantWindows[qidx].ResizeWindow(160*2, 120*2)
		quadrantWindows[qidx].MoveWindow(int(2*160*(qidx%2)), int(2*120*(qidx/2)))
		quadrantWindows[qidx].IMShow(resultImg)
	}

	// Update the histograms and recalculate the color models with the colors from the dots
	if nbCircleMasks == 4 {
		isAllCircleMasksSeen = true
		for qidx := 0; qidx < len(quadrants); qidx++ {
			sampleColorWithMask(frame, circleMasks, colorHistSums, nonColorHistSums, qidx)

			*nbHistsSampled++

			if *nbHistsSampled > MIN_NB_COLOR_SAMPLES*3 {
				isSheetCalibrated = true
			}
			if *nbHistsSampled%50 == 0 {
				calcBayesColorModel(colorHistSums[qidx], nonColorHistSums[qidx], &colorModels[qidx])
			}

			frameMasked := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8UC3)
			defer frameMasked.Close()
			frameMasked.SetTo(gocv.NewScalar(float64(cornerColors[qidx].B),
				float64(cornerColors[qidx].G),
				float64(cornerColors[qidx].R),
				float64(cornerColors[qidx].A)))
			frameMasked.CopyToWithMask(canvas, circleMasks[qidx])
		}
	}

	return isAllCircleMasksSeen, isSheetCalibrated
}

// Draw 3D axes on the checkerboard
func drawAxes(canvas *gocv.Mat, axesVector gocv.Point3fVector, mtx, dist, rvec, tvec gocv.Mat) {
	// Draw the axes
	projAxesPoints := gocv.NewPoint2fVector()
	defer projAxesPoints.Close()
	jacobian := gocv.NewMat()
	defer jacobian.Close()
	gocv.ProjectPoints(axesVector, rvec, tvec, mtx, dist, projAxesPoints, &jacobian, 0)

	// Draw the axes
	axesPoints := projAxesPoints.ToPoints()
	pt0 := image.Pt(int(axesPoints[0].X), int(axesPoints[0].Y))
	ax_pt1 := image.Pt(int(axesPoints[1].X), int(axesPoints[1].Y))
	ax_pt2 := image.Pt(int(axesPoints[2].X), int(axesPoints[2].Y))
	ax_pt3 := image.Pt(int(axesPoints[3].X), int(axesPoints[3].Y))

	gocv.Line(canvas, pt0, ax_pt1, colorRed, 5)
	gocv.Line(canvas, pt0, ax_pt2, colorGreen, 5)
	gocv.Line(canvas, pt0, ax_pt3, colorBlue, 5)
}

// Advanced writing on the canvas that actually looks nice
func prettyPutText(canvas *gocv.Mat, text string, origin image.Point, color color.RGBA, fontScale float64) {
	fontFace := gocv.FontHersheySimplex
	fontThickness := 1
	lineType := gocv.LineAA
	gocv.GetTextSize(text, fontFace, fontScale, fontThickness)
	gocv.PutTextWithParams(canvas, text, origin, fontFace, fontScale, colorBlack, fontThickness+2, lineType, false)
	gocv.PutTextWithParams(canvas, text, origin, fontFace, fontScale, color, fontThickness, lineType, false)
}

func calcStraightChessboard(frame gocv.Mat, cornerDotVector gocv.Point3fVector, cbCheckersR3 gocv.Point3fVector,
	mtx, dist, rvec, tvec gocv.Mat, termCriteria gocv.TermCriteria) straightChessboard {
	srcSize := image.Pt(frame.Cols(), frame.Rows())
	dstSize := image.Pt(int(1298), int(700))

	fmt.Println("RVEC")
	PrintMatValues64FC(rvec)
	fmt.Println("TVEC")
	PrintMatValues64FC(tvec)
	newCamMatr, roi := gocv.GetOptimalNewCameraMatrixWithParams(mtx, dist, srcSize, 1, srcSize, false)

	M := gocv.NewMat()
	mapx := gocv.NewMat()
	mapy := gocv.NewMat()

	r := gocv.NewMat()
	defer r.Close()
	gocv.InitUndistortRectifyMap(mtx, dist, r, newCamMatr, srcSize, 5, &mapx, &mapy)
	canvas := gocv.NewMat()
	defer canvas.Close()
	gocv.Remap(frame, &canvas, mapx, mapy, gocv.InterpolationLinear, gocv.BorderConstant, colorBlack)
	cbRegion := canvas.Region(roi)
	defer cbRegion.Close()
	regionGray := gocv.NewMat()
	defer regionGray.Close()
	gocv.CvtColor(cbRegion, &regionGray, gocv.ColorBGRToGray)
	regionCorners := gocv.NewMat()
	defer regionCorners.Close()
	found := gocv.FindChessboardCorners(regionGray, image.Pt(W, H), &regionCorners, gocv.CalibCBAdaptiveThresh+gocv.CalibCBFastCheck)

	if found {
		gocv.CornerSubPix(regionGray, &regionCorners, image.Pt(11, 11), image.Pt(-1, -1), termCriteria)
		regionCanvas := cbRegion.Clone()
		defer regionCanvas.Close()
		gocv.DrawChessboardCorners(&regionCanvas, image.Pt(W, H), regionCorners, found)

		point2fVector := gocv.NewPoint2fVectorFromMat(regionCorners)
		defer point2fVector.Close()
		region_rvec := gocv.NewMat()
		defer region_rvec.Close()
		region_tvec := gocv.NewMat()
		defer region_tvec.Close()
		inliers := gocv.NewMat()
		defer inliers.Close()
		gocv.SolvePnPRansac(cbCheckersR3, point2fVector, mtx, dist, &region_rvec, &region_tvec, false, 100, 8, 0.99, &inliers, 0)

		projPoints := gocv.NewPoint2fVector()
		defer projPoints.Close()
		jacobian := gocv.NewMat()
		defer jacobian.Close()
		chessboardCornersR3 := gocv.NewPoint3fVector()
		defer chessboardCornersR3.Close()
		chessboardCornersR3.Append(cornerDotVector.At(2))
		chessboardCornersR3.Append(cornerDotVector.At(5))
		chessboardCornersR3.Append(cornerDotVector.At(11))
		chessboardCornersR3.Append(cornerDotVector.At(12))
		gocv.ProjectPoints(chessboardCornersR3, region_rvec, region_tvec, mtx, dist, projPoints, &jacobian, 0)

		chessboardCornersR2Points := []gocv.Point2f{}
		chessboardCornersR2Points = append(chessboardCornersR2Points, gocv.NewPoint2f(0, 0))
		chessboardCornersR2Points = append(chessboardCornersR2Points, gocv.NewPoint2f(float32(dstSize.X-1), 0))
		chessboardCornersR2Points = append(chessboardCornersR2Points, gocv.NewPoint2f(0, float32(dstSize.Y-1)))
		chessboardCornersR2Points = append(chessboardCornersR2Points, gocv.NewPoint2f(float32(dstSize.X-1), float32(dstSize.Y-1)))
		chessboardCornersR2 := gocv.NewPoint2fVectorFromPoints(chessboardCornersR2Points)
		defer chessboardCornersR2.Close()

		M.Close()
		M = gocv.GetPerspectiveTransform2f(projPoints, chessboardCornersR2)
		// scbRegion := gocv.NewMat()
		// defer scbRegion.Close()
		// gocv.WarpPerspective(cbRegion, &scbRegion, M, dstSize)
		// gocv.NewWindow("straightened").IMShow(scbRegion)
	}

	return straightChessboard{
		mapx: mapx,
		mapy: mapy,
		roi:  roi,
		M:    M,
	}
}

func straightenChessboard(frame gocv.Mat, sc straightChessboard) gocv.Mat {
	mapx := sc.mapx
	mapy := sc.mapy
	roi := sc.roi
	M := sc.M
	dstSize := image.Pt(int(1298), int(700))

	canvas := gocv.NewMat()
	defer canvas.Close()

	gocv.Remap(frame, &canvas, mapx, mapy, gocv.InterpolationLinear, gocv.BorderConstant, colorBlack)

	cbRegion := canvas.Region(roi)
	defer cbRegion.Close()

	scbRegion := gocv.NewMat()
	gocv.WarpPerspective(cbRegion, &scbRegion, M, dstSize)

	return scbRegion
}

func detectColors(frame gocv.Mat, colorModels []gocv.Mat, masks []*Deque, colorSpaceFactor float64) {
	dotsMask := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8U)
	defer dotsMask.Close()

	for cidx, colorModel := range colorModels {
		mask := predictCheckerColor(frame, colorModel, colorSpaceFactor)
		maskClone := mask.Clone()
		masks[cidx].Push(&maskClone)
		mask.Close()

		if masks[cidx].Size() >= 3 {
			masksSum := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8U)
			defer masksSum.Close()
			for maskPast := range masks[cidx].Iter() {
				gocv.Add(*maskPast, masksSum, &masksSum)
			}

			gocv.BitwiseOr(dotsMask, masksSum, &dotsMask)

			color := gocv.NewScalar(
				float64(cornerColors[cidx].B),
				float64(cornerColors[cidx].G),
				float64(cornerColors[cidx].R),
				255,
			)

			frameColor := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8UC3)
			defer frameColor.Close()
			frameColor.SetTo(color)
			frameColor.CopyToWithMask(&frame, masksSum)
		}
	}

	findCornerDots(dotsMask)

	gocv.NewWindow("Detection").IMShow(frame)
}

// Main calibration function that calibrates the webcam projection space and the checker color models
func chessBoardCalibration(webcam *gocv.VideoCapture, debugwindow, projection *gocv.Window) calibrationResults {
	debugwindow.ResizeWindow(1024, 768)

	nbChkBrdFound := 0
	nbHistsSampled := 0
	colorSpaceFactor := 0.0

	isCalibrated := false
	isModeled := false
	isSheetCalibrated := false

	// unlockWebcam()
	// isLocked := false
	isLocked := true

	CHK_W := float32(W)
	CHK_H := float32(H)
	cornerDots := [][]float32{
		{0, 0, 0}, // red LT
		{0, -1, 0},
		{-1, -1, 0},
		{-1, 0, 0},
		{CHK_W, 0, 0}, // green RT
		{CHK_W, -1, 0},
		{CHK_W - 1, -1, 0},
		{CHK_W - 1, 0, 0},
		{0, CHK_H, 0}, // purple LB
		{0, CHK_H - 1, 0},
		{-1, CHK_H - 1, 0},
		{-1, CHK_H, 0},
		{CHK_W, CHK_H, 0}, // orange RB
		{CHK_W, CHK_H - 1, 0},
		{CHK_W - 1, CHK_H - 1, 0},
		{CHK_W - 1, CHK_H, 0},
	}
	cornerDotVector := gocv.NewPoint3fVector()
	defer cornerDotVector.Close()
	for i := 0; i < len(cornerDots); i++ {
		point := gocv.NewPoint3f(cornerDots[i][0], cornerDots[i][1], cornerDots[i][2])
		cornerDotVector.Append(point)
	}

	axesPoints := [][]float32{
		{0, 0, 0},
		{3, 0, 0},
		{0, 3, 0},
		{0, 0, -3},
	}
	axesVector := gocv.NewPoint3fVector()
	defer axesVector.Close()
	for i := 0; i < len(axesPoints); i++ {
		point := gocv.NewPoint3f(axesPoints[i][0], axesPoints[i][1], axesPoints[i][2])
		axesVector.Append(point)
	}

	// prepare object points, like (0,0,0), (1,0,0), (2,0,0) ....,(6,5,0)
	cbCornerPoints3f := gocv.NewPoints3fVector()
	defer cbCornerPoints3f.Close()
	cbCornerPoints2f := gocv.NewPoints2fVector()
	defer cbCornerPoints2f.Close()

	objp := make([][]float32, W*H)
	for i := range objp {
		objp[i] = make([]float32, 3)
	}

	cbCheckersR3 := gocv.NewPoint3fVector()
	defer cbCheckersR3.Close()
	for i := 0; i < H; i++ {
		for j := 0; j < W; j++ {
			objp[i*W+j][0] = float32(j) // X
			objp[i*W+j][1] = float32(i) // Y
			objp[i*W+j][2] = float32(0) // Z, always 0 because the chessboard is flat

			point := gocv.NewPoint3f(objp[i*W+j][0], objp[i*W+j][1], objp[i*W+j][2])
			cbCheckersR3.Append(point)
		}
	}
	points2f := []gocv.Point2f{}
	for i := 0; i < H; i++ {
		for j := 0; j < W; j++ {
			objp[i*W+j][0] = float32(j) // X
			objp[i*W+j][1] = float32(i) // Y

			point := gocv.NewPoint2f(objp[i*W+j][0], objp[i*W+j][1])
			points2f = append(points2f, point)
		}
	}
	cbCheckersR2 := gocv.NewPoint2fVectorFromPoints(points2f)
	defer cbCheckersR2.Close()

	img := gocv.NewMat()
	defer img.Close()

	cimg := gocv.NewMatWithSize(beamerHeight, beamerWidth, gocv.MatTypeCV8UC3)

	fi := frameInput{
		webcam:      webcam,
		debugWindow: debugwindow,
		projection:  projection,
		img:         img,
		cimg:        cimg,
	}

	termCriteria := gocv.NewTermCriteria(gocv.MaxIter+gocv.EPS, 30, 0.001)

	mtx := gocv.NewMat()
	defer mtx.Close()
	dist := gocv.NewMat()
	defer dist.Close()
	rvecs := gocv.NewMat()
	defer rvecs.Close()
	tvecs := gocv.NewMat()
	defer tvecs.Close()

	// cornersWindow := gocv.NewWindow("corners")
	// defer cornersWindow.Close()
	quadrantWindows := [4]*gocv.Window{}
	for qidx := 0; qidx < 4; qidx++ {
		quadrantWindows[qidx] = gocv.NewWindow(fmt.Sprintf("Quadrant %d", qidx))
	}

	saturation := 70
	value := 47
	trackbarsS := debugwindow.CreateTrackbar("Saturation", 255)
	trackbarsS.SetPos(saturation)
	trackbarsV := debugwindow.CreateTrackbar("Value", 255)
	trackbarsV.SetPos(value)

	colorHistSums := make([]gocv.Mat, NB_CLRD_CHCKRS)
	nonColorHistSums := make([]gocv.Mat, NB_CLRD_CHCKRS)
	colorModels := make([]gocv.Mat, NB_CLRD_CHCKRS)
	for i := 0; i < NB_CLRD_CHCKRS; i++ {
		colorHistSums[i] = gocv.NewMat()
		nonColorHistSums[i] = gocv.NewMat()
		colorModels[i] = gocv.NewMat()
	}

	for i := 0; i < 4; i++ {
		deque := NewDeque(5) // Initialize each deque with a capacity
		masksGlob = append(masksGlob, deque)
	}
	masks := []*Deque{}
	for i := 0; i < 4; i++ {
		deque := NewDeque(3) // Initialize each deque with a capacity
		masks = append(masks, deque)
	}

	scChsBrd := straightChessboard{}

	for {
		start := time.Now()
		if ok := fi.webcam.Read(&fi.img); !ok {
			break
		}
		if fi.img.Empty() {
			continue
		}

		frame := fi.img.Clone()
		defer frame.Close()
		frameRegion := gocv.NewMat()
		defer frameRegion.Close()

		// Find the chess board corners
		corners := gocv.NewMat()
		defer corners.Close()
		isChkbrdFound := false

		if !isModeled {
			// convert the rgb frame into gray
			frameGray := gocv.NewMat()
			defer frameGray.Close()

			gocv.CvtColor(frame, &frameGray, gocv.ColorBGRToGray)
			isChkbrdFound = gocv.FindChessboardCorners(frameGray, image.Pt(W, H), &corners, gocv.CalibCBAdaptiveThresh+gocv.CalibCBFastCheck)

			if isChkbrdFound {
				gocv.CornerSubPix(frameGray, &corners, image.Pt(11, 11), image.Pt(-1, -1), termCriteria)
			}
		}

		var fndStr string
		if isChkbrdFound {
			fndStr = "CHKBRD FOUND"
		} else {
			fndStr = "CHKBRD NOT FOUND"
		}

		rvec := gocv.NewMat()
		defer rvec.Close()
		tvec := gocv.NewMat()
		defer tvec.Close()

		if isChkbrdFound && !isCalibrated {
			cbCornerPoints3f.Append(cbCheckersR3)
			point2fVector := gocv.NewPoint2fVectorFromMat(corners)
			defer point2fVector.Close()
			cbCornerPoints2f.Append(point2fVector)

			nbChkBrdFound++

			if nbChkBrdFound >= MIN_NB_CHKBRD_FOUND {
				srcSize := image.Pt(fi.img.Cols(), fi.img.Rows())
				reprojError := gocv.CalibrateCamera(cbCornerPoints3f, cbCornerPoints2f, srcSize, &mtx, &dist, &rvecs, &tvecs, gocv.CalibFlag(gocv.CalibCBFastCheck))
				numVecs := rvecs.Rows()
				rvec := rvecs.Row(numVecs - 1)
				tvec := tvecs.Row(numVecs - 1)
				lastRvec := rvec.Clone()
				defer lastRvec.Close()
				lastTvec := tvec.Clone()
				defer lastTvec.Close()

				scChsBrd = calcStraightChessboard(frame, cornerDotVector, cbCheckersR3, mtx, dist, lastRvec, lastTvec, termCriteria)
				fmt.Println("ROI", scChsBrd.roi)
				fmt.Println(scChsBrd.mapx.Rows(), scChsBrd.mapy.Cols())

				fmt.Println("=== Calibrated! === Reprojection error:", reprojError)
				isCalibrated = true
			}

		} else if isChkbrdFound && isCalibrated {
			point2fVector := gocv.NewPoint2fVectorFromMat(corners)
			defer point2fVector.Close()
			inliers := gocv.NewMat()
			defer inliers.Close()
			gocv.SolvePnPRansac(cbCheckersR3, point2fVector, mtx, dist, &rvec, &tvec, false, 100, 8, 0.99, &inliers, 0)

			if !isModeled {
				cornerPointsProj := projectCornerPoints(cornerDotVector, mtx, dist, rvec, tvec)
				cornersWindow := gocv.NewWindow("corners")
				sampleColors(frame, cornerPointsProj, colorHistSums, nonColorHistSums, cornersWindow)

				nbHistsSampled++

				if nbHistsSampled > MIN_NB_COLOR_SAMPLES {
					for cidx := 0; cidx < len(colorModels); cidx++ {
						colorSpaceFactor, _ = calcBayesColorModel(colorHistSums[cidx], nonColorHistSums[cidx], &colorModels[cidx])
						fmt.Println("=== Checker colors modeled! ===")
					}

					isModeled = true
					cornersWindow.Close()
				}
			}
		}

		if isCalibrated {
			frameRegion = straightenChessboard(frame, scChsBrd)
		}

		isAllCircleMasksSeen := false
		if isModeled && !isSheetCalibrated {
			nbFRcts, rectangles := predictCheckerColors(frame, &fi.img, cornerColors, colorModels, colorSpaceFactor)
			quadrants := [4][4]image.Point{}

			if nbFRcts >= 4 {
				// Draw some lines to visualize the quadrants
				lt := rectangles[0].Min
				rt := image.Pt(rectangles[1].Max.X, rectangles[1].Min.Y)
				lb := image.Pt(rectangles[2].Min.X, rectangles[2].Max.Y)
				rb := rectangles[3].Max

				mt := image.Point{
					X: (lt.X + rt.X) / 2,
					Y: (lt.Y + rt.Y) / 2,
				}
				ml := image.Point{
					X: (lt.X + lb.X) / 2,
					Y: (lt.Y + lb.Y) / 2,
				}
				mr := image.Point{
					X: (rt.X + rb.X) / 2,
					Y: (rt.Y + rb.Y) / 2,
				}
				mb := image.Point{
					X: (lb.X + rb.X) / 2,
					Y: (lb.Y + rb.Y) / 2,
				}
				c := image.Point{
					X: (lt.X + rb.X) / 2,
					Y: (lt.Y + rb.Y) / 2,
				}

				gocv.Line(&fi.img, lt, rt, colorBlack, 1)
				gocv.Line(&fi.img, rt, rb, colorBlack, 1)
				gocv.Line(&fi.img, rb, lb, colorBlack, 1)
				gocv.Line(&fi.img, lb, lt, colorBlack, 1)
				gocv.Line(&fi.img, mt, mb, colorBlack, 1)
				gocv.Line(&fi.img, ml, mr, colorBlack, 1)

				quadrants[0] = [4]image.Point{
					lt, mt, c, ml,
				}
				quadrants[1] = [4]image.Point{
					mt, rt, mr, c,
				}
				quadrants[2] = [4]image.Point{
					ml, c, mb, lb,
				}
				quadrants[3] = [4]image.Point{
					c, mr, rb, mb,
				}

				isAllCircleMasksSeen, isSheetCalibrated = calibrateSheet(frame, &fi.img, quadrants, colorHistSums, nonColorHistSums, colorModels, &nbHistsSampled,
					quadrantWindows, trackbarsS, trackbarsV)

				if isSheetCalibrated {
					for qidx := range quadrantWindows {
						quadrantWindows[qidx].Close()
					}
					fmt.Println("=== Sheet calibrated! ===")
				}

			} else {
				fmt.Println("Not enough rectangles found: ", nbFRcts)
			}

		} else if isSheetCalibrated && !frameRegion.Empty() {
			// Use color models on straightened chessboard to detect the colors
			detectColors(frameRegion, colorModels, masks, colorSpaceFactor)
		}

		if isCalibrated && !isLocked {
			// lockWebcam(exposureTime, whiteBalanceTemperature)
			isLocked = true
		}

		if !rvec.Empty() {
			// Draw the axes
			drawAxes(&fi.img, axesVector, mtx, dist, rvec, tvec)

		} else {
			// Draw and display the corners
			gocv.DrawChessboardCorners(&fi.img, image.Pt(W, H), corners, isChkbrdFound)
		}

		fps := time.Second / time.Since(start)
		exposureTime := getWebcamExposureTime()
		whiteBalanceTemperature := getWebcamwhiteBalanceTemperature()

		prettyPutText(&fi.img, fmt.Sprintf("FPS: %d", fps), image.Pt(10, 15), colorWhite, 0.4)
		prettyPutText(&fi.img, fmt.Sprintf("%s (%d, %d)", fndStr, nbChkBrdFound, nbHistsSampled), image.Pt(10, 30), colorWhite, 0.4)
		prettyPutText(&fi.img, fmt.Sprintf("ExposureTime: %d, WhiteBalanceTemp.: %d", exposureTime, whiteBalanceTemperature), image.Pt(10, 45), colorWhite, 0.4)
		prettyPutText(&fi.img, fmt.Sprintf("isCalibrated: %t, isLocked: %t, isModeled: %t, isSheetCalibrated: %t", isCalibrated, isLocked, isModeled, isSheetCalibrated), image.Pt(10, 60), colorWhite, 0.4)

		if !isCalibrated {
			color := colorRed
			if isChkbrdFound {
				color = colorGreen
			}
			prettyPutText(&fi.img, "Place the checkerboard", image.Pt(10, 75), color, 0.3)

		} else if isCalibrated && !isModeled {
			prettyPutText(&fi.img, "Sampling colors ...", image.Pt(10, 75), colorGreen, 0.3)

		} else if isModeled && !isSheetCalibrated {
			color := colorRed
			if isAllCircleMasksSeen {
				color = colorGreen
			}
			prettyPutText(&fi.img, "Adjust the sliders until you see the four checkers in the four quadrant windows,", image.Pt(10, 75), color, 0.3)
			prettyPutText(&fi.img, "Then, hold the calibration sheet in the middle", image.Pt(10, 85), color, 0.3)
			prettyPutText(&fi.img, "and align each calibration color to each quadrant color", image.Pt(10, 95), color, 0.3)
		}

		fi.debugWindow.IMShow(fi.img)
		fi.projection.IMShow(fi.cimg)
		key := fi.debugWindow.WaitKey(WAIT)
		if key == 27 {
			break
		} else if key == 113 {
			nbHistsSampled = 0
			isModeled = false
			isSheetCalibrated = false
		}
		if key >= 0 {
			fmt.Println("Key:", key)
		}
	}

	for cidx := 0; cidx < len(colorModels); cidx++ {
		colorModels[cidx].Close()
		colorHistSums[cidx].Close()
		nonColorHistSums[cidx].Close()
	}

	scChsBrd.M.Close()
	scChsBrd.mapx.Close()
	scChsBrd.mapy.Close()

	return calibrationResults{
		rvec: rvecs,
		tvec: tvecs,
		mtx:  mtx,
		dist: dist,
	}
}
