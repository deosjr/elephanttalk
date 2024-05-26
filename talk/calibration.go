package talk

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"log"
	"sync"
	"time"
	"unsafe"

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

const EPSILON = 2.220446049250313e-16 // epsilon for division by zero prevention

var colorWhite = color.RGBA{255, 255, 255, 255}
var colorRed = color.RGBA{255, 0, 0, 255}
var colorGreen = color.RGBA{0, 255, 0, 255}
var colorBlue = color.RGBA{0, 0, 255, 255}
var cornerColors = []color.RGBA{
	{56, 158, 13, 255},  // green
	{250, 140, 22, 255}, // orange
	{57, 16, 133, 255},  // purple
	{245, 34, 45, 255},  // red
}

// var cornerColors = []color.RGBA{
// 	{255, 0, 0, 255}, // red
// 	{255, 0, 0, 255}, // red
// 	{255, 0, 0, 255}, // red
// 	{255, 0, 0, 255}, // red
// }

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
			val = EPSILON
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

// func divideMatByScalar(mat gocv.Mat, scalar float64) (gocv.Mat, error) {
// 	if mat.Type() != gocv.MatTypeCV32F {
// 		return gocv.Mat{}, fmt.Errorf("mat is not of type CV_32F")
// 	}

// 	data := mat.ToBytes()
// 	outData := make([]byte, len(data))
// 	for i := 0; i < len(data); i += 4 {
// 		bits := binary.LittleEndian.Uint32(data[i : i+4])
// 		val := *(*float32)(unsafe.Pointer(&bits))
// 		newVal := float32(val) / float32(scalar)
// 		binary.LittleEndian.PutUint32(outData[i:i+4], *(*uint32)(unsafe.Pointer(&newVal)))
// 	}

// 	// Create a new Mat with the same dimensions as the input but with the modified data
// 	result, err := gocv.NewMatWithSizesFromBytes(mat.Size(), gocv.MatTypeCV32F, outData)
// 	if err != nil {
// 		return gocv.Mat{}, err
// 	}

// 	return result, nil
// }

// Print3DMatValues prints all the values of a 3D gocv.Mat in a structured tabular format.
func Print3DMatValues32f(mat gocv.Mat) {
	// Assume mat is a 3D matrix where each point is a single float
	rows := mat.Size()[0]
	cols := mat.Size()[1]
	depth := mat.Size()[2] // Assuming the third dimension size is retrievable like this

	for d := 0; d < depth; d++ {
		fmt.Printf("Slice %d:\n", d)
		for c := 0; c < cols; c++ {
			for r := 0; r < rows; r++ {
				val := mat.GetFloatAt3(d, c, r)
				fmt.Printf("%9.8f ", val)
			}
			fmt.Println() // New line for each row
		}
		fmt.Println() // Extra new line after each depth slice
	}
}
func Print3DMatValues32i(mat gocv.Mat) {
	// Assume mat is a 3D matrix where each point is a single float
	rows := mat.Rows()
	cols := mat.Cols()

	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			val := mat.GetIntAt(r, c)
			fmt.Printf("%d ", val)
		}
		fmt.Println() // New line for each row
	}
}
func Print3DMatValues8UC3(mat gocv.Mat) {
	// Assume mat is a 3D matrix where each point is a single float
	rows := mat.Size()[0]
	cols := mat.Size()[1]
	chns := mat.Channels()
	mat_data, _ := mat.DataPtrUint8()
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			fmt.Printf("[")
			for ch := 0; ch < chns; ch++ {
				val := mat_data[mat.Cols()*r*3+c*3+ch]
				fmt.Printf("%d ", val)
			}
			fmt.Println("]")
		}
		fmt.Println()
	}
	fmt.Println() // Extra new line after each depth slice
}

func calcChanceDistribution(cSumHist gocv.Mat, ncSumHist gocv.Mat, pRGBColorChance *gocv.Mat) (float64, error) {
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

	gocv.Divide(pRgbColorMul, pRGB, pRGBColorChance) // P(checker_color|rgb) : Bayes' theorem
	// gocv.Normalize(*pRGBColorChance, pRGBColorChance, 0, 1, gocv.NormMinMax)
	minValpRGBColorChance, maxValpRGBColorChance, _, _ := gocv.MinMaxIdx(*pRGBColorChance)
	sumPRGBColorChance, _ := sumMat(*pRGBColorChance)
	fmt.Println("MinVal", minValpRGBColorChance, "MaxVal", maxValpRGBColorChance, "Sum", sumPRGBColorChance)
	// Print3DMatValues32f(*pRGBColorChance)

	// Calculate the scaling factor for mapping the frame RGB color dimensions (256x256x256) to
	// histogram color space dimensions (eg.: 32x32x32)
	// Given that we have 256 colors in three channels, we map each of the three dimensions to the 32x32x32 color space
	// So the probability of a pixel belonging to the checker color comes from looking in pRGBColorChance at the
	// given RGB color's location in the 32x32x32 3D chance distribution
	colorSpaceFactor := 1.0 / 256.0 * float64(pRGBColorChance.Size()[0])
	fmt.Println("colorSpaceFactor", colorSpaceFactor)

	return colorSpaceFactor, nil
}

func predictCheckerColors(frame gocv.Mat, canvas *gocv.Mat, cornerColors []color.RGBA, colorModels []gocv.Mat, colorSpaceFactor float64, theta float64) {
	frameFloat := gocv.NewMat()
	defer frameFloat.Close()
	frame.ConvertTo(&frameFloat, gocv.MatTypeCV32FC3)
	// frame_red := frame.Clone()
	// defer frame_red.Close()
	// red := gocv.NewScalar(0, 0, 255, 0)
	// frame_red.SetTo(red)
	// frame_red.ConvertTo(&frameFloat, gocv.MatTypeCV32FC3)
	// fmt.Println("Size", frameFloat.Size(), "Channels", frameFloat.Channels(), "Type", frameFloat.Type())
	csFactorMat := gocv.NewMatFromScalar(gocv.NewScalar(colorSpaceFactor, colorSpaceFactor, colorSpaceFactor, colorSpaceFactor), gocv.MatTypeCV64F)
	// fmt.Println("Size", csFactorMat.Size(), "Channels", csFactorMat.Channels(), "Type", csFactorMat.Type())
	defer csFactorMat.Close()
	csIndices := gocv.NewMat()
	defer csIndices.Close()
	// frameFloat has all RGB colors (256x256x256), scale and save each color as an index in histogram (32x32x32)
	// color space with dimensions of the webcam frame into a pixel->colorspace lookup table (csIndices)
	gocv.Multiply(frameFloat, csFactorMat, &csIndices)
	csIndices.ConvertTo(&csIndices, gocv.MatTypeCV16S)

	// fmt.Println("Size", csIndices.Size(), "Channels", csIndices.Channels(), "Type", csIndices.Type())
	// mincsIndices, maxcsIndices, _, _ := gocv.MinMaxLoc(csIndices)
	// fmt.Println("MinVal", mincsIndices, "MaxVal", maxcsIndices)
	// Print3DMatValues32i(csIndices)
	// ir := int(csIndices.GetIntAt(0, 0*3+2))
	// Print3DMatValues32f(colorModels[0])
	// fmt.Println(ir, colorModels[0].GetFloatAt3(ir, 0, 0))

	// canvasData, _ := canvas.DataPtrUint8()
	// sliceSze := canvas.Cols() * canvas.Channels()
	// nbChnls := canvas.Channels()

	// for _, colorModel := range colorModels {
	// 	// Iterate over each color location
	// 	for y := 0; y < csIndices.Rows(); y++ {
	// 		for x := 0; x < csIndices.Cols(); x++ {
	// 			ib := int(csIndices.GetShortAt(y, x*3+0)) // Blue channel for x index
	// 			ig := int(csIndices.GetShortAt(y, x*3+1)) // Green channel for y index
	// 			ir := int(csIndices.GetShortAt(y, x*3+2)) // Red channel (not used for indexing but exemplified)

	// 			// Calculate probability for the given model and indices
	// 			prob := float64(colorModel.GetFloatAt3(ib, ig, ir))
	// 			if prob > theta {
	// 				canvasData[y*sliceSze+x*nbChnls+0] = colorWhite.B
	// 				canvasData[y*sliceSze+x*nbChnls+1] = colorWhite.G
	// 				canvasData[y*sliceSze+x*nbChnls+2] = colorWhite.R
	// 			}
	// 		}
	// 	}
	// }

	var wg sync.WaitGroup
	for pidx, colorModel := range colorModels {
		wg.Add(1)
		go func(pidx int, colorModel gocv.Mat) {
			defer wg.Done()

			mask := gocv.NewMat()
			defer mask.Close()
			mask = gocv.NewMatWithSize(canvas.Rows(), canvas.Cols(), gocv.MatTypeCV8U)

			// Iterate over each color location
			for y := 0; y < csIndices.Rows(); y++ {
				for x := 0; x < csIndices.Cols(); x++ {
					ib := int(csIndices.GetShortAt(y, x*3+0)) // Blue channel
					ig := int(csIndices.GetShortAt(y, x*3+1)) // Green channel
					ir := int(csIndices.GetShortAt(y, x*3+2)) // Red channel

					// Lookup probability of the given color belonging to the given checker color
					// with chance of at least theta
					if float64(colorModel.GetFloatAt3(ib, ig, ir)) > theta {
						mask.SetUCharAt(y, x, 255)
					}
				}
			}

			// Erode and dilate the mask to remove noise
			gocv.MorphologyEx(mask, &mask, gocv.MorphErode, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(3, 3)))
			gocv.MorphologyEx(mask, &mask, gocv.MorphDilate, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(5, 5)))

			frame_color := gocv.NewMatWithSize(canvas.Rows(), canvas.Cols(), gocv.MatTypeCV8UC3)
			defer frame_color.Close()

			color := gocv.NewScalar(
				float64(cornerColors[pidx].B),
				float64(cornerColors[pidx].G),
				float64(cornerColors[pidx].R),
				255,
			)
			frame_color.SetTo(color)
			frame_color.CopyToWithMask(canvas, mask)

		}(pidx, colorModel)
	}
	wg.Wait()
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
	pt0 := image.Pt(int(axesPoints[0].X), int(axesPoints[0].Y))
	ax_pt1 := image.Pt(int(axesPoints[1].X), int(axesPoints[1].Y))
	ax_pt2 := image.Pt(int(axesPoints[2].X), int(axesPoints[2].Y))
	ax_pt3 := image.Pt(int(axesPoints[3].X), int(axesPoints[3].Y))

	gocv.Line(canvas, pt0, ax_pt1, colorRed, 5)
	gocv.Line(canvas, pt0, ax_pt2, colorGreen, 5)
	gocv.Line(canvas, pt0, ax_pt3, colorBlue, 5)
}

func chessBoardCalibration(webcam *gocv.VideoCapture, debugwindow, projection *gocv.Window) calibrationResults {
	// // Create a 3x3 image with 3 channels for RGB data
	// img := gocv.NewMatWithSizesWithScalar(
	// 	[]int{4, 4},
	// 	gocv.MatTypeCV8UC3,
	// 	gocv.NewScalar(0, 0, 0, 0),
	// )
	// defer img.Close()
	// fmt.Println("Rows", img.Rows(), "Cols", img.Cols(), "Channels", img.Channels())
	// // Print3DMatValues8UC3(&img)

	// // Define RGB values for each pixel
	// // For simplicity, each color component will increment across the image
	// img_data, _ := img.DataPtrUint8()

	// for r := 0; r < img.Rows(); r++ {
	// 	for c := 0; c < img.Cols(); c++ {
	// 		img_data[img.Cols()*r*3+c*3+0] = 0   // uint8(25 * (r + c + 1))
	// 		img_data[img.Cols()*r*3+c*3+1] = 0   // uint8(25*(r+c+1) + 10)
	// 		img_data[img.Cols()*r*3+c*3+2] = 255 // uint8(25*(r+c+1) + 20)

	// 		// // Increment color value for visualization
	// 		// value0 := uint8(25 * (r + c + 1))
	// 		// value1 := value0 + 10
	// 		// value2 := value0 + 20
	// 		// // Set the BGR values (note OpenCV uses BGR by default, not RGB)
	// 		// img.SetUCharAt3(0, c, r, value0)
	// 		// img.SetUCharAt3(1, c, r, value1)
	// 		// img.SetUCharAt3(2, c, r, value2)

	// 		// // val0 := img.GetUCharAt3(0, j, i)
	// 		// // val1 := img.GetUCharAt3(1, j, i)
	// 		// // val2 := img.GetUCharAt3(2, j, i)
	// 		// // fmt.Printf("[%d %d %d], ", val0, val1, val2)
	// 		// // fmt.Println()
	// 	}
	// 	// fmt.Println()
	// }
	// img_data, _ = img.DataPtrUint8()
	// // fmt.Println(img_data)
	// img1, _ := gocv.NewMatFromBytes(4, 4, gocv.MatTypeCV8UC3, img_data)
	// Print3DMatValues8UC3(img1)

	// hist := gocv.NewMat()
	// defer hist.Close()
	// gocv.CalcHist([]gocv.Mat{img1}, []int{0, 1, 2}, gocv.NewMat(), &hist, []int{3, 3, 3}, []float64{0, 256, 0, 256, 0, 256}, true)
	// Print3DMatValues32f(hist)

	// fmt.Println("Hist", hist.Size(), hist.Channels(), hist.Type())
	// for x := 0; x < hist.Size()[0]; x++ {
	// 	for y := 0; y < hist.Size()[1]; y++ {
	// 		for z := 0; z < hist.Size()[2]; z++ {
	// 			fmt.Printf("%9.8f ", hist.GetFloatAt3(x, y, z))
	// 		}
	// 	}
	// 	fmt.Println()
	// }
	// prob := hist.GetFloatAt3(0, 0, 2)
	// fmt.Println("Prob", prob)

	// return calibrationResults{}

	const W = 13             // nb checkers on board horizontal
	const H = 6              // nb checkers on board vertical
	const CW = 100           // checker projected resolution (W)
	const CH = 100           // checker projected resolution (H)
	const CB = 10            // checker projected resolution boundary
	const NB_CLRD_CHCKRS = 4 // number of colored checkers

	// Curiously enough the Go code does not work with histograms of 32x32x32 but only with histograms > 128x128x128
	const HIST_SIZE = 129          // color histogram bins per dimension
	const THETA = 0.5              // probability threshold for color prediction
	const MIN_NB_CHKBRD_FOUND = 20 // minimum number of frames with checkerboard found
	const MIN_COLOR_SAMPLES = 20   // minimum number of color samples for color models
	const WAIT = 5                 // wait time in milliseconds (don't make it too large)

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

	// frame0Window := gocv.NewWindow("frame0Window")
	// defer frame0Window.Close()
	// frame1Window := gocv.NewWindow("frame1Window")
	// defer frame1Window.Close()

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

					// Invert the mask
					maskInv := gocv.NewMat()
					defer maskInv.Close()
					gocv.BitwiseNot(mask, &maskInv)

					// frame_red := frame.Clone()
					// defer frame_red.Close()
					// red := gocv.NewScalar(0, 0, 255, 0)
					// frame_red.SetTo(red)

					// frame_red_nor := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8UC3)
					// defer frame_red_nor.Close()
					// frame_red.CopyToWithMask(&frame_red_nor, mask)

					// frame_red_inv := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8UC3)
					// defer frame_red_inv.Close()
					// frame_red.CopyToWithMask(&frame_red_inv, maskInv)

					// frame0Window.IMShow(frame_red_nor)
					// frame1Window.IMShow(frame_red_inv)
					// min_frn, max_frn, _, _ := gocv.MinMaxLoc(frame_red_nor)
					// min_fri, max_fri, _, _ := gocv.MinMaxLoc(frame_red_inv)
					// fmt.Println("MinVal", min_frn, "MaxVal", max_frn, "MinVal", min_fri, "MaxVal", max_fri)
					// fmt.Println("Size", frame_red.Size(), "Channels", frame_red.Channels(), "Type", frame_red.Type())

					colorHist := gocv.NewMat()
					defer colorHist.Close()
					gocv.CalcHist([]gocv.Mat{frame}, []int{0, 1, 2}, mask, &colorHist,
						[]int{HIST_SIZE, HIST_SIZE, HIST_SIZE}, []float64{0, 256, 0, 256, 0, 256}, false)

					nonColorHist := gocv.NewMat()
					defer nonColorHist.Close()
					gocv.CalcHist([]gocv.Mat{frame}, []int{0, 1, 2}, maskInv, &nonColorHist,
						[]int{HIST_SIZE, HIST_SIZE, HIST_SIZE}, []float64{0, 256, 0, 256, 0, 256}, false)

					// colorHistR := gocv.NewMat()
					// defer colorHistR.Close()
					// gocv.CalcHist([]gocv.Mat{frame}, []int{2}, mask, &colorHistR,
					// 	[]int{HIST_SIZE}, []float64{0, 256}, false)
					// minVal, maxVal, _, _ := gocv.MinMaxIdx(colorHistR)
					// fmt.Println("MinVal", minVal, "MaxVal", maxVal)
					// sumcolorHistR, _ := sumMat(colorHistR)
					// fmt.Println("pRgbColorMul", colorHistR.Size(), "sumPRgbColorMul", sumcolorHistR)

					if colorHistSums[cidx].Empty() {
						colorHistSums[cidx] = colorHist.Clone()

					} else {
						gocv.Add(colorHistSums[cidx], colorHist, &colorHistSums[cidx])
					}

					if nonColorHistSums[cidx].Empty() {
						nonColorHistSums[cidx] = nonColorHist.Clone()

					} else {
						gocv.Add(nonColorHistSums[cidx], nonColorHist, &nonColorHistSums[cidx])
					}

					if nbHistsSampled > MIN_COLOR_SAMPLES {
						colorSpaceFactor, _ = calcChanceDistribution(colorHistSums[cidx], nonColorHistSums[cidx], &colorModels[cidx])
						isModeled = true
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

				nbHistsSampled++
			}
		}

		if isModeled {
			predictCheckerColors(frame, &fi.img, cornerColors, colorModels, colorSpaceFactor, THETA)
			// fi.img, _ = gocv.NewMatFromBytes(fi.img.Rows(), fi.img.Cols(), gocv.MatTypeCV8UC3, canvas_data)
			// defer fi.img.Close()
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
