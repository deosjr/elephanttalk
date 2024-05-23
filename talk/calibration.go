package talk

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"log"
	"time"
	"unsafe"

	"gocv.io/x/gocv"
)

type calibrationResults struct {
	pixelsPerCM float64
	// displacement    point
	displayRatio    float64
	referenceColors []color.RGBA
	rvec            gocv.Mat
	tvec            gocv.Mat
	mtx             gocv.Mat
	dist            gocv.Mat
}

const EPSILON = 1e-10 // epsilon for division by zero

var colorWhite = color.RGBA{255, 255, 255, 255}
var colorRed = color.RGBA{255, 0, 0, 255}
var colorGreen = color.RGBA{0, 255, 0, 255}
var colorBlue = color.RGBA{0, 0, 255, 255}
var cornerColors = []color.RGBA{
	{13, 158, 56, 0},  // green
	{22, 140, 250, 0}, // orange
	{133, 16, 57, 0},  // purple
	{45, 34, 245, 0},  // red
}

func MatToFloat32Slice(mat gocv.Mat) gocv.Mat {
	if mat.Empty() {
		return mat
	}

	totalElements := mat.Total()
	channels := mat.Channels()

	data := make([][]float32, mat.Total())
	for i := 0; i < mat.Total(); i++ {
		data[i] = make([]float32, mat.Channels())
	}

	mat.ConvertTo(&mat, gocv.MatTypeCV64FC3)
	for i := 0; i < totalElements; i++ {
		values := mat.GetVecdAt(i, 0)
		for j := 0; j < channels; j++ {
			data[i][j] = float32(values[j])
		}
	}

	out := gocv.NewMatWithSize(1, 3, gocv.MatTypeCV32FC3)
	out.SetFloatAt(0, 0, data[len(data)-1][0])
	out.SetFloatAt(0, 1, data[len(data)-1][1])
	out.SetFloatAt(0, 2, data[len(data)-1][2])

	return out
}

func sumMat(mat gocv.Mat) (float64, error) {
	if mat.Type() != gocv.MatTypeCV32F {
		return 0, fmt.Errorf("mat is not of type CV_32F")
	}

	data := mat.ToBytes()
	var sum float64
	for i := 0; i < len(data); i += 4 {
		bits := binary.LittleEndian.Uint32(data[i : i+4])
		val := *(*float32)(unsafe.Pointer(&bits))
		sum += float64(val)
	}
	return sum, nil
}

func ensureNonZero(mat gocv.Mat) (gocv.Mat, error) {
	if mat.Type() != gocv.MatTypeCV32F {
		return gocv.Mat{}, fmt.Errorf("mat is not of type CV_32F")
	}

	data := mat.ToBytes()
	outData := make([]byte, len(data))
	for i := 0; i < len(data); i += 4 {
		bits := binary.LittleEndian.Uint32(data[i : i+4])
		val := *(*float32)(unsafe.Pointer(&bits))
		if val == 0 {
			val = float32(EPSILON)
		}
		binary.LittleEndian.PutUint32(outData[i:i+4], *(*uint32)(unsafe.Pointer(&val)))
	}

	// Create a new Mat with the same dimensions as the input but with the modified data
	result, err := gocv.NewMatWithSizesFromBytes(mat.Size(), gocv.MatTypeCV32F, outData)
	if err != nil {
		return gocv.Mat{}, err
	}

	return result, nil
}

func divideMatByScalar(mat gocv.Mat, scalar float64) (gocv.Mat, error) {
	if mat.Type() != gocv.MatTypeCV32F {
		return gocv.Mat{}, fmt.Errorf("mat is not of type CV_32F")
	}

	data := mat.ToBytes()
	outData := make([]byte, len(data))
	for i := 0; i < len(data); i += 4 {
		bits := binary.LittleEndian.Uint32(data[i : i+4])
		val := *(*float32)(unsafe.Pointer(&bits))
		newVal := float32(val) / float32(scalar)
		binary.LittleEndian.PutUint32(outData[i:i+4], *(*uint32)(unsafe.Pointer(&newVal)))
	}

	// Create a new Mat with the same dimensions as the input but with the modified data
	result, err := gocv.NewMatWithSizesFromBytes(mat.Size(), gocv.MatTypeCV32F, outData)
	if err != nil {
		return gocv.Mat{}, err
	}

	return result, nil
}

func calcChanceDistribution(cSumHist gocv.Mat, ncSumHist gocv.Mat, pRGBColorChance *gocv.Mat) (float64, error) {
	if cSumHist.Empty() || ncSumHist.Empty() {
		log.Fatal("colorHistSums is empty")
		return 0.0, fmt.Errorf("invalid mat")
	}

	cHistSumSc, _ := sumMat(cSumHist)   // Total sum of the colored checker
	ncHistSumSc, _ := sumMat(ncSumHist) // Total sum of the non-colored checker

	// Hit it Bayes!
	pColor := cHistSumSc / (cHistSumSc + ncHistSumSc) // Chance of hitting the given colored checker, given a random color
	pNonColor := 1 - pColor                           // Chance of not hitting the given colored checker, given a random color

	pRgbColor, _ := divideMatByScalar(cSumHist, cHistSumSc)      // Chance distribution in color space of belonging to colored checker
	pRgbNonColor, _ := divideMatByScalar(ncSumHist, ncHistSumSc) // Chance distribution in color space of not belonging to colored checker

	pRgb := gocv.NewMat()
	defer pRgb.Close()
	gocv.AddWeighted(pRgbColor, pColor, pRgbNonColor, pNonColor, 0, &pRgb) // Sum and weigh chance distribution color/non-color
	pRgb, _ = ensureNonZero(pRgb)                                          // Make sure we don't divide by zero

	// Add the color model to the list
	pRgbColorMul := gocv.NewMat()
	defer pRgbColorMul.Close()
	pColorMat := gocv.NewMatFromScalar(gocv.NewScalar(pColor, 0, 0, 0), gocv.MatTypeCV64F)
	defer pColorMat.Close()

	gocv.Multiply(pRgbColor, pColorMat, &pRgbColorMul)
	gocv.Divide(pRgbColorMul, pRgb, pRGBColorChance) // Normalize the color model to give the probability of a pixel belonging to the checker color

	// Calculate the scaling factor for mapping the frame RGB color dimensions (256x256x256) to
	// histogram color space dimensions (32x32x32)
	// Given that we have 256 colors in three channels, we map each of the three dimensions to the 32x32x32 color space
	// So the probability of a pixel belonging to the checker color comes from looking in pRGBColorChance at the
	// given RGB color's location in the 32x32x32 3D chance distribution
	colorSpaceFactor := 1.0 / 256.0 * float64(pRGBColorChance.Size()[0])

	return colorSpaceFactor, nil
}

func predictCheckerColors(frame gocv.Mat, canvas *gocv.Mat, cornerColors []color.RGBA, colorModels []gocv.Mat, colorSpaceFactor float64, theta float64) {
	frameFloat := gocv.NewMat()
	defer frameFloat.Close()
	frame.ConvertTo(&frameFloat, gocv.MatTypeCV32F)
	csFactorMat := gocv.NewMatFromScalar(gocv.NewScalar(colorSpaceFactor, 0, 0, 0), gocv.MatTypeCV64F)
	defer csFactorMat.Close()
	csIndices := gocv.NewMat()
	defer csIndices.Close()
	// frameFloat has all RGB colors (256x256x256), scale and save each color as an index in (32x32x32)
	// color space with dimensions of the webcam frame into a pixel->colorspace lookup table (csIndices)
	gocv.Multiply(frameFloat, csFactorMat, &csIndices)
	csIndices.ConvertTo(&csIndices, gocv.MatTypeCV32S)

	for cidx, cornerColor := range cornerColors {
		// Iterate over each color location
		for y := 0; y < csIndices.Rows(); y++ {
			for x := 0; x < csIndices.Cols(); x++ {
				ix := int(csIndices.GetIntAt(y, 3*x))   // Blue channel for x index
				iy := int(csIndices.GetIntAt(y, 3*x+1)) // Green channel for y index
				iz := int(csIndices.GetIntAt(y, 3*x+2)) // Red channel (not used for indexing but exemplified)

				// Assuming colorModels[cidx] can be accessed similarly; this might need customization
				prob := float64(colorModels[cidx].GetFloatAt3(ix, iy, iz))

				if prob > theta {
					canvas.SetUCharAt(y, x*3+0, cornerColor.R)
					canvas.SetUCharAt(y, x*3+1, cornerColor.G)
					canvas.SetUCharAt(y, x*3+2, cornerColor.B)
				}
			}
		}
	}
}

func drawAxes(canvas *gocv.Mat, axesVector gocv.Point3fVector, rvec gocv.Mat, tvec gocv.Mat, mtx gocv.Mat, dist gocv.Mat) {
	// Draw the axes
	projAxesPoints := gocv.NewPoint2fVector()
	defer projAxesPoints.Close()
	jacobian := gocv.NewMat()
	defer jacobian.Close()
	gocv.ProjectPoints(axesVector, rvec, tvec, mtx, dist, projAxesPoints, &jacobian, 0)

	// Draw the axes
	axesPoints := projAxesPoints.ToPoints()
	pt1 := image.Pt(int(axesPoints[0].X), int(axesPoints[0].Y))
	pt2 := image.Pt(int(axesPoints[1].X), int(axesPoints[1].Y))
	pt3 := image.Pt(int(axesPoints[2].X), int(axesPoints[2].Y))
	pt4 := image.Pt(int(axesPoints[3].X), int(axesPoints[3].Y))

	gocv.Line(canvas, pt1, pt2, colorRed, 5)
	gocv.Line(canvas, pt1, pt3, colorGreen, 5)
	gocv.Line(canvas, pt1, pt4, colorBlue, 5)
}

func chessBoardCalibration(webcam *gocv.VideoCapture, debugwindow, projection *gocv.Window) calibrationResults {
	const W = 13             // nb checkers on board horizontal
	const H = 6              // nb checkers on board vertical
	const CW = 100           // checker projected resolution (W)
	const CH = 100           // checker projected resolution (H)
	const CB = 10            // checker projected resolution boundary
	const NB_CLRD_CHCKRS = 4 // number of colored checkers

	const HIST_SIZE = 32           // color histogram bins per dimension
	const THETA = 0.33             // probability threshold for color prediction
	const MIN_NB_CHKBRD_FOUND = 10 // minimum number of frames with checkerboard found
	const MIN_COLOR_SAMPLES = 10   // minimum number of color samples for color models
	const WAIT = 100

	nbChkBrdFound := 0
	nbHistsSampled := 0
	colorSpaceFactor := 0.0

	isCalibrated := false
	isModeled := false

	CHK_W := float32(W)
	CHK_H := float32(H)
	cornerDots := [][]float32{
		{0, 0, 0}, // red LT
		{0, -1, 0},
		{-1, -1, 0},
		{-1, 0, 0},
		{CHK_W, 0, 0}, // purple RT
		{CHK_W, -1, 0},
		{CHK_W - 1, -1, 0},
		{CHK_W - 1, 0, 0},
		{0, CHK_H, 0}, // orange LB
		{0, CHK_H - 1, 0},
		{-1, CHK_H - 1, 0},
		{-1, CHK_H, 0},
		{CHK_W, CHK_H, 0}, // green RB
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
	objectPoints := gocv.NewPoints3fVector()
	defer objectPoints.Close()
	imgPoints := gocv.NewPoints2fVector()
	defer imgPoints.Close()

	objp := make([][]float32, W*H)
	for i := range objp {
		objp[i] = make([]float32, 3)
	}

	p3fv := gocv.NewPoint3fVector()
	defer p3fv.Close()
	for i := 0; i < H; i++ {
		for j := 0; j < W; j++ {
			objp[i*W+j][0] = float32(j) // X
			objp[i*W+j][1] = float32(i) // Y
			objp[i*W+j][2] = float32(0) // Z, always 0 because the chessboard is flat

			point := gocv.NewPoint3f(objp[i*W+j][0], objp[i*W+j][1], objp[i*W+j][2])
			p3fv.Append(point)
		}
	}

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

	cornersWindow := gocv.NewWindow("corners")
	defer cornersWindow.Close()

	colorHistSums := make([]gocv.Mat, NB_CLRD_CHCKRS)
	nonColorHistSums := make([]gocv.Mat, NB_CLRD_CHCKRS)
	colorModels := make([]gocv.Mat, NB_CLRD_CHCKRS)
	for i := 0; i < NB_CLRD_CHCKRS; i++ {
		colorHistSums[i] = gocv.NewMat()
		defer colorHistSums[i].Close()
		nonColorHistSums[i] = gocv.NewMat()
		defer nonColorHistSums[i].Close()
		colorModels[i] = gocv.NewMat()
		defer colorModels[i].Close()
	}

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
		frameGray := gocv.NewMat()
		defer frameGray.Close()

		// convert the rgb frame into gray
		gocv.CvtColor(frame, &frameGray, gocv.ColorBGRToGray)

		// Find the chess board corners
		corners := gocv.NewMat()
		defer corners.Close()
		found := gocv.FindChessboardCorners(frameGray, image.Pt(W, H), &corners, gocv.CalibCBAdaptiveThresh+gocv.CalibCBFastCheck)

		var fndStr string
		if found {
			fndStr = "FOUND"
		} else {
			fndStr = "NOT FOUND"
		}

		if found && !isCalibrated {
			objectPoints.Append(p3fv)

			gocv.CornerSubPix(frameGray, &corners, image.Pt(11, 11), image.Pt(-1, -1), termCriteria)
			point2fVector := gocv.NewPoint2fVectorFromMat(corners)
			defer point2fVector.Close()
			imgPoints.Append(point2fVector)

			nbChkBrdFound++
		}

		rvec := gocv.NewMat()
		defer rvec.Close()
		tvec := gocv.NewMat()
		defer tvec.Close()

		if found && isCalibrated {
			gocv.CornerSubPix(frameGray, &corners, image.Pt(11, 11), image.Pt(-1, -1), termCriteria)

			point2fVector := gocv.NewPoint2fVectorFromMat(corners)
			defer point2fVector.Close()
			inliers := gocv.NewMat()
			defer inliers.Close()

			gocv.SolvePnPRansac(p3fv, point2fVector, mtx, dist, &rvec, &tvec, false, 100, 8, 0.99, &inliers, 0)

			if !isModeled {
				projPoints := gocv.NewPoint2fVector()
				defer projPoints.Close()
				jacobian := gocv.NewMat()
				defer jacobian.Close()

				gocv.ProjectPoints(cornerDotVector, rvec, tvec, mtx, dist, projPoints, &jacobian, 0)

				for i := 0; i < projPoints.Size(); i++ {
					point := projPoints.At(i)
					imagePoint := image.Pt(int(point.X), int(point.Y))
					gocv.Circle(&fi.cimg, imagePoint, 4, colorWhite, 2)
				}

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

				finalCrops := gocv.NewMat()
				defer finalCrops.Close()

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

					if colorModels[cidx].Empty() {
						colorHist := gocv.NewMat()
						defer colorHist.Close()
						gocv.CalcHist([]gocv.Mat{frame}, []int{0, 1, 2}, mask, &colorHist,
							[]int{HIST_SIZE, HIST_SIZE, HIST_SIZE}, []float64{0, 256, 0, 256, 0, 256}, false)

						if colorHistSums[cidx].Empty() {
							// fmt.Println("Size", colorHist.Size(), "Type", colorHist.Type(), "Channels", colorHist.Channels())
							colorHistSums[cidx] = colorHist.Clone()
						} else {
							gocv.Add(colorHistSums[cidx], colorHist, &colorHistSums[cidx])
						}

						// Invert the mask
						maskInv := gocv.NewMat()
						defer maskInv.Close()
						gocv.BitwiseNot(mask, &maskInv)

						nonColorHist := gocv.NewMat()
						defer nonColorHist.Close()
						gocv.CalcHist([]gocv.Mat{frame}, []int{0, 1, 2}, maskInv, &nonColorHist,
							[]int{HIST_SIZE, HIST_SIZE, HIST_SIZE}, []float64{0, 256, 0, 256, 0, 256}, false)

						if nonColorHistSums[cidx].Empty() {
							nonColorHistSums[cidx] = nonColorHist.Clone()
						} else {
							gocv.Add(nonColorHistSums[cidx], nonColorHist, &nonColorHistSums[cidx])
						}

						if nbHistsSampled > MIN_COLOR_SAMPLES {
							colorSpaceFactor, _ = calcChanceDistribution(colorHistSums[cidx], nonColorHistSums[cidx], &colorModels[cidx])
						}
					}

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
						finalCrops = finalCrop.Clone()
					} else {
						gocv.Vconcat(finalCrops, finalCrop, &finalCrops)
					}
				}

				cornersWindow.IMShow(finalCrops)

				if nbHistsSampled > MIN_COLOR_SAMPLES {
					isModeled = true
				}

				nbHistsSampled++
			}
		}

		if isModeled {
			predictCheckerColors(frame, &fi.img, cornerColors, colorModels, colorSpaceFactor, THETA)
		}

		gocv.PutText(&fi.img, fmt.Sprintf("%s (%d, %d)", fndStr, nbChkBrdFound, nbHistsSampled), image.Pt(0, 40), 0, .5, colorRed, 2)

		if found && !isCalibrated && nbChkBrdFound >= MIN_NB_CHKBRD_FOUND {
			reproj_err := gocv.CalibrateCamera(objectPoints, imgPoints, image.Pt(W, H), &mtx, &dist, &rvecs, &tvecs, 0)
			fmt.Println("=== Calibrated! === Reprojection error:", reproj_err)
			isCalibrated = true
		}

		if !rvec.Empty() {
			// Draw the axes
			drawAxes(&fi.img, axesVector, rvec, tvec, mtx, dist)

		} else {
			// Draw and display the corners
			gocv.DrawChessboardCorners(&fi.img, image.Pt(W, H), corners, found)
		}

		fps := time.Second / time.Since(start)
		gocv.PutText(&fi.img, fmt.Sprintf("FPS: %d", fps), image.Pt(0, 20), 0, .5, colorGreen, 2)

		fi.debugWindow.IMShow(fi.img)
		fi.projection.IMShow(fi.cimg)
		key := fi.debugWindow.WaitKey(WAIT)
		if key >= 0 {
			cornersWindow.Close()
			break
		}
	}

	return calibrationResults{
		rvec: MatToFloat32Slice(rvecs),
		tvec: MatToFloat32Slice(tvecs),
		mtx:  mtx,
		dist: dist,
	}
}
