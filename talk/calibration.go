package talk

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os/exec"
	"strconv"
	"strings"
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

// Curiously enough the Go code does not work with histograms of 32x32x32 but only with histograms > 128x128x128
const W = 12                    // nb checkers on board horizontal
const H = 6                     // nb checkers on board vertical
const HIST_SIZE = 160           // color histogram bins per dimension
const THETA = 0.25              // probability threshold for color prediction
const MIN_NB_CHKBRD_FOUND = 50  // minimum number of frames with checkerboard found
const MIN_NB_COLOR_SAMPLES = 50 // minimum number of color samples for color models

const NB_CLRD_CHCKRS = 4              // number of colored checkers
const CW = 100                        // checker projected resolution (W)
const CH = 100                        // checker projected resolution (H)
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
	{245, 34, 45, 255},         // red
	{56, 158, 13, 255},         // green
	{57 * 2, 16 * 2, 133, 255}, // purple
	{250, 140, 22, 255},        // orange
}

var masks = []*Deque{}

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

func getMaskArea(mask gocv.Mat) float64 {
	area := gocv.CountNonZero(mask)
	return float64(area)
}

func getContourArea(contour gocv.PointVector) float64 {
	area := gocv.ContourArea(contour)
	return area
}

func getCenter(points []image.Point) image.Point {
	// Convert the slice of Points to a Mat
	contourMat_cidx00 := gocv.NewMatWithSize(1, len(points), gocv.MatTypeCV32SC2)
	defer contourMat_cidx00.Close()
	for i, point := range points {
		contourMat_cidx00.SetIntAt(0, i*2, int32(point.X))
		contourMat_cidx00.SetIntAt(0, i*2+1, int32(point.Y))
	}
	// gocv.FillPoly(&contourMat_cidx00, [][]image.Point{points}, colorWhite)

	// Calculate the moments of the contour
	moments := gocv.Moments(contourMat_cidx00, false)

	// Calculate the centroid using the moments
	cx := moments["m10"] / moments["m00"]
	cy := moments["m01"] / moments["m00"]
	return image.Pt(int(cx), int(cy))
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

func unlockWebcam() {
	// v4l2-ctl --device /dev/video0 -c auto_exposure=3 -c white_balance_automatic=1
	cmd := exec.Command("v4l2-ctl", "--device", "/dev/video0", "-c", "auto_exposure=3", "-c", "white_balance_automatic=1")
	output, err := cmd.Output()

	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Command output: %s\n", output)
}

func lockWebcam(exposureTime, whiteBalanceTemperature int) {
	// v4l2-ctl --device /dev/video0 -c auto_exposure=1 -c exposure_time_absolute=305 -c white_balance_automatic=0 -c white_balance_temperature=8000
	cmd := exec.Command("v4l2-ctl", "--device", "/dev/video0",
		"-c", "auto_exposure=1",
		"-c", fmt.Sprintf("exposure_time_absolute=%d", exposureTime),
		"-c", "white_balance_automatic=0",
		"-c", fmt.Sprintf("white_balance_temperature=%d", whiteBalanceTemperature))
	output, err := cmd.Output()

	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Command output: %s\n", output)
}

func getWebcamExposureTime() int {
	cmd := exec.Command("v4l2-ctl", "--device", "/dev/video0", "-C", "exposure_time_absolute")
	output, err := cmd.Output()

	if err != nil {
		log.Fatal(err)
		return -1
	}

	parts := strings.Split(string(output), ":")
	exposureTime, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		fmt.Println("Error:", err)
		return -1
	}

	return exposureTime
}

func getWebcamwhiteBalanceTemperature() int {
	cmd := exec.Command("v4l2-ctl", "--device", "/dev/video0", "-C", "white_balance_temperature")
	output, err := cmd.Output()

	if err != nil {
		log.Fatal(err)
		return -1
	}

	parts := strings.Split(string(output), ":")
	whiteBalanceTemperature, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		fmt.Println("Error:", err)
		return -1
	}

	return whiteBalanceTemperature
}

// Print3DMatValues prints all the values of a 3D gocv.Mat in a structured tabular format
func PrintMatValues64F(mat gocv.Mat) {
	// Assume mat is a 3D matrix where each point is a single float
	rows := mat.Rows()
	cols := mat.Cols()

	fmt.Println("Rows", rows, "Cols", cols)
	fmt.Println("Channels", mat.Channels(), "Type", mat.Type())

	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			val := mat.GetDoubleAt(r, c)
			fmt.Printf("%9.8f ", val)
		}
		fmt.Println() // New line for each row
	}
}

func PrintMatValues64FC(mat gocv.Mat) {
	// Assume mat is a 3D matrix where each point is a single float
	rows := mat.Rows()
	cols := mat.Cols()
	chns := mat.Channels()

	fmt.Println("Rows", rows, "Cols", cols)
	fmt.Println("Channels", chns, "Type", mat.Type())

	mat_data, _ := mat.DataPtrFloat64()
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			fmt.Printf("[")
			for ch := 0; ch < chns; ch++ {
				val := mat_data[mat.Cols()*r*3+c*3+ch]
				fmt.Printf("%9.8f ", val)
			}
			fmt.Println("]")
		}
		fmt.Println() // New line for each row
	}
}

func PrintMatValues32FC3(mat gocv.Mat) {
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
func PrintMatValues32I(mat gocv.Mat) {
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
func PrintMatValues8UC3(mat gocv.Mat) {
	// Assume mat is a 3D matrix where each point is a single float
	rows := mat.Size()[0]
	cols := mat.Size()[1]
	chns := mat.Channels()

	fmt.Println("Rows", rows, "Cols", cols)
	fmt.Println("Channels", chns, "Type", mat.Type())

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

func PrintMatValues8U(mat gocv.Mat) {
	// Assume mat is a 3D matrix where each point is a single float
	rows := mat.Rows()
	cols := mat.Cols()
	chns := mat.Channels()

	fmt.Println("Rows", rows, "Cols", cols)
	fmt.Println("Channels", chns, "Type", mat.Type())

	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			val := mat.GetUCharAt(r, c)
			fmt.Printf("%d ", val)
		}
		fmt.Println() // New line for each row
	}
}

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

	// colorHistR := gocv.NewMat()
	// defer colorHistR.Close()
	// gocv.CalcHist([]gocv.Mat{frame}, []int{2}, mask, &colorHistR,
	// 	[]int{HIST_SIZE}, []float64{0, 256}, false)
	// minVal, maxVal, _, _ := gocv.MinMaxIdx(colorHistR)
	// fmt.Println("MinVal", minVal, "MaxVal", maxVal)
	// sumcolorHistR, _ := sumMat(colorHistR)
	// fmt.Println("pRgbColorMul", colorHistR.Size(), "sumPRgbColorMul", sumcolorHistR)

	if colorHistSums[cidx].Empty() {
		colorHistSums[cidx].Close()
		colorHistSums[cidx] = colorHist.Clone()

	} else {
		// colorHistSum, _ := sumMat(colorHistSums[cidx])
		// fmt.Println(cidx, "colorHistSum", colorHistSum)
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
		gocv.CalcHist([]gocv.Mat{frame_clrsp}, []int{0, 1, 2}, mask, &colorHist,
			[]int{HIST_SIZE, HIST_SIZE, HIST_SIZE}, []float64{0, 256, 0, 256, 0, 256}, false)

		nonColorHist := gocv.NewMat()
		gocv.CalcHist([]gocv.Mat{frame_clrsp}, []int{0, 1, 2}, maskInv, &nonColorHist,
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
			colorHistSums[cidx].Close()
			colorHistSums[cidx] = colorHist.Clone()

		} else {
			// colorHistSum, _ := sumMat(colorHistSums[cidx])
			// fmt.Println(cidx, "colorHistSum", colorHistSum)
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

func predictCheckerColor(frame gocv.Mat, colorModel gocv.Mat, colorSpaceFactor float64) gocv.Mat {
	mask := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8U)

	frame_clrsp := gocv.NewMat()
	defer frame_clrsp.Close()
	gocv.CvtColor(frame, &frame_clrsp, gocv.ColorBGRToLuv)

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
	csIndices.ConvertTo(&csIndices, gocv.MatTypeCV16S)

	// Iterate over each color location
	for y := 0; y < csIndices.Rows(); y++ {
		for x := 0; x < csIndices.Cols(); x++ {
			ib := int(csIndices.GetShortAt(y, x*3+0)) // Blue channel
			ig := int(csIndices.GetShortAt(y, x*3+1)) // Green channel
			ir := int(csIndices.GetShortAt(y, x*3+2)) // Red channel

			// Lookup probability of the given color belonging to the given checker color
			// with chance of at least theta
			if float64(colorModel.GetFloatAt3(ib, ig, ir)) > THETA {
				mask.SetUCharAt(y, x, 255)
			}
		}
	}

	// Erode and dilate the mask to remove noise
	gocv.MorphologyEx(mask, &mask, gocv.MorphErode, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(3, 3)))
	gocv.MorphologyEx(mask, &mask, gocv.MorphDilate, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(5, 5)))

	return mask
}

type RectanglePoint struct {
	Rect   image.Rectangle
	Center image.Point
}

func getExtremePoint(rectanglePoints []RectanglePoint, pidx int) RectanglePoint {
	topLeft, topRight, bottomLeft, bottomRight := rectanglePoints[0], rectanglePoints[0], rectanglePoints[0], rectanglePoints[0]

	for _, rp := range rectanglePoints {
		if rp.Center.X < topLeft.Center.X || (rp.Center.X == topLeft.Center.X && rp.Center.Y < topLeft.Center.Y) {
			topLeft = rp
		}
		if rp.Center.X > topRight.Center.X || (rp.Center.X == topRight.Center.X && rp.Center.Y < topRight.Center.Y) {
			topRight = rp
		}
		if rp.Center.X < bottomLeft.Center.X || (rp.Center.X == bottomLeft.Center.X && rp.Center.Y > bottomLeft.Center.Y) {
			bottomLeft = rp
		}
		if rp.Center.X > bottomRight.Center.X || (rp.Center.X > bottomRight.Center.X && rp.Center.Y > bottomRight.Center.Y) {
			bottomRight = rp
		}
	}

	if pidx == 0 {
		return topLeft
	} else if pidx == 1 {
		return topRight
	} else if pidx == 2 {
		return bottomLeft
	} else {
		return bottomRight
	}
}

func predictCheckerColors(frame gocv.Mat, canvas *gocv.Mat,
	cornerColors []color.RGBA, colorModels []gocv.Mat, colorSpaceFactor float64) (int, [4]image.Rectangle) {

	frame_clrsp := gocv.NewMat()
	defer frame_clrsp.Close()
	gocv.CvtColor(frame, &frame_clrsp, gocv.ColorBGRToLuv)

	frameFloat := gocv.NewMat()
	defer frameFloat.Close()
	frame_clrsp.ConvertTo(&frameFloat, gocv.MatTypeCV32FC3)
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
	nbFoundRects := 0
	checkerRectangles := [4]image.Rectangle{}

	var wg sync.WaitGroup
	for pidx, colorModel := range colorModels {
		wg.Add(1)
		go func(pidx int, colorModel gocv.Mat) {
			defer wg.Done()

			mask := gocv.NewMatWithSize(canvas.Rows(), canvas.Cols(), gocv.MatTypeCV8U)

			// Iterate over each color location
			for y := 0; y < csIndices.Rows(); y++ {
				for x := 0; x < csIndices.Cols(); x++ {
					ib := int(csIndices.GetShortAt(y, x*3+0)) // Blue channel
					ig := int(csIndices.GetShortAt(y, x*3+1)) // Green channel
					ir := int(csIndices.GetShortAt(y, x*3+2)) // Red channel

					// Lookup probability of the given color belonging to the given checker color
					// with chance of at least theta
					if float64(colorModel.GetFloatAt3(ib, ig, ir)) > THETA {
						mask.SetUCharAt(y, x, 255)
					}
				}
			}

			// Erode and dilate the mask to remove noise
			gocv.MorphologyEx(mask, &mask, gocv.MorphErode, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(3, 3)))
			gocv.MorphologyEx(mask, &mask, gocv.MorphDilate, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(5, 5)))
			mask_clone := mask.Clone()
			masks[pidx].Push(&mask_clone)
			mask.Close()

			frameColor := gocv.NewMatWithSize(canvas.Rows(), canvas.Cols(), gocv.MatTypeCV8UC3)
			defer frameColor.Close()

			if masks[pidx].Size() >= 5 {
				masksSum := gocv.NewMatWithSize(canvas.Rows(), canvas.Cols(), gocv.MatTypeCV8U)
				defer masksSum.Close()
				for maskPast := range masks[pidx].Iter() {
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

				contoursVector := gocv.FindContours(masksSum, gocv.RetrievalTree, gocv.ChainApproxSimple)
				rectanglePoints := []RectanglePoint{}
				for _, contour := range contoursVector.ToPoints() {
					contourVector := gocv.NewPointVectorFromPoints(contour)
					defer contourVector.Close()
					if getContourArea(contourVector) > 750 {
						rectanglePoints = append(rectanglePoints, RectanglePoint{
							Rect:   gocv.BoundingRect(contourVector),
							Center: getCenter(contour),
						})
					}
				}

				if len(rectanglePoints) > 0 {
					extremePoint := getExtremePoint(rectanglePoints, pidx)
					gocv.RectangleWithParams(canvas, extremePoint.Rect, colorBlack, 2, gocv.LineAA, 0)
					checkerRectangles[pidx] = extremePoint.Rect
					nbFoundRects++
				}
			}

		}(pidx, colorModel)
	}
	wg.Wait()

	return nbFoundRects, checkerRectangles
}

func lineLength(a, b image.Point) float64 {
	return math.Sqrt(float64((b.X-a.X)*(b.X-a.X) + (b.Y-a.Y)*(b.Y-a.Y)))
}

func calibrateSheet(frame gocv.Mat, canvas *gocv.Mat, quadrants [4][4]image.Point,
	colorHistSums, nonColorHistSums, colorModels []gocv.Mat, nbHistsSampled *int,
	quadrantWindows [4]*gocv.Window, trackbarsS *gocv.Trackbar, trackbarsV *gocv.Trackbar) bool {

	isSheetCalibrated := false

	circleMasks := [4]gocv.Mat{}
	nbCircleMasks := 0
	for qidx, quadrant := range quadrants {
		cw := int(lineLength(quadrant[0], quadrant[1]))
		ch := int(lineLength(quadrant[1], quadrant[2]))

		pointsVector := gocv.NewPointsVector()
		defer pointsVector.Close()
		projPointsArr := []gocv.Point2f{}
		imagePoints := gocv.NewPointVector()
		defer imagePoints.Close()
		for _, imagePoint := range quadrant {
			point2f := gocv.Point2f{X: float32(imagePoint.X), Y: float32(imagePoint.Y)}
			projPointsArr = append(projPointsArr, point2f)
			imagePoints.Append(imagePoint)
		}
		pointsVector.Append(imagePoints)

		mask := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8U)
		defer mask.Close()
		gocv.FillPoly(&mask, pointsVector, colorWhite)

		poly := gocv.NewMat()
		defer poly.Close()
		gocv.BitwiseAndWithMask(frame, frame, &poly, mask)

		p2sPlane := []gocv.Point2f{{X: 0, Y: 0}, {X: float32(cw), Y: 0}, {X: float32(cw), Y: float32(ch)}, {X: 0, Y: float32(ch)}}

		projPtsV := gocv.NewPoint2fVectorFromPoints(projPointsArr)
		defer projPtsV.Close()
		planePtsV := gocv.NewPoint2fVectorFromPoints(p2sPlane)
		defer planePtsV.Close()

		M := gocv.GetPerspectiveTransform2f(projPtsV, planePtsV)
		defer M.Close()

		tPoly := gocv.NewMat()
		defer tPoly.Close()
		gocv.WarpPerspective(poly, &tPoly, M, image.Pt(cw, ch))

		// Convert BGR to HSV
		hsvImg := gocv.NewMat()
		defer hsvImg.Close()
		gocv.CvtColor(tPoly, &hsvImg, gocv.ColorBGRToHSV)

		// Create a mask for red colors
		// Note: Adjust the hue values to better suit the exact tone of red
		cornerColor := cornerColors[qidx]
		cornerColorMat := gocv.NewMatWithSize(1, 1, gocv.MatTypeCV8UC3)
		cornerColorMat.SetTo(gocv.NewScalar(float64(cornerColor.B), float64(cornerColor.G), float64(cornerColor.R), 0))
		gocv.CvtColor(cornerColorMat, &cornerColorMat, gocv.ColorBGRToHSV)
		saturation := float64(trackbarsS.GetPos())
		value := float64(trackbarsV.GetPos())

		lbMask0 := gocv.NewScalar(0, saturation, value, 0)
		ubMask0 := gocv.NewScalar(180, 255, 255, 0)

		maskChecker := gocv.NewMat()
		defer maskChecker.Close()
		gocv.InRangeWithScalar(hsvImg, lbMask0, ubMask0, &maskChecker)

		// Erode and dilate the mask to remove noise
		gocv.MorphologyEx(maskChecker, &maskChecker, gocv.MorphErode, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(3, 3)))
		gocv.MorphologyEx(maskChecker, &maskChecker, gocv.MorphDilate, gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(5, 5)))

		// Apply the mask to the original image
		resultImg := gocv.NewMat()
		defer resultImg.Close()
		tPoly.CopyToWithMask(&resultImg, maskChecker)

		colorHists := []gocv.Mat{}
		contours := gocv.FindContours(maskChecker, gocv.RetrievalTree, gocv.ChainApproxSimple)
		defer contours.Close()
		contoursVector := gocv.NewPointsVector()
		defer contoursVector.Close()
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

		if len(colorHists) > 1 {
			maxDist := float32(-100.0)
			cidx00 := 0
			cidx01 := 0
			for cidx0 := 0; cidx0 < len(colorHists); cidx0++ {
				for cidx1 := cidx0 + 1; cidx1 < len(colorHists); cidx1++ {
					colorHist0 := colorHists[cidx0]
					colorHist1 := colorHists[cidx1]

					dist := gocv.CompareHist(colorHist0, colorHist1, gocv.HistCmpCorrel)
					if dist > maxDist {
						maxDist = dist
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
			if qidx == 0 {
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
			} else if qidx == 1 {
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
			} else if qidx == 2 {
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
			} else if qidx == 3 {
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

			circleMaskFrameW := gocv.NewMat()
			gocv.WarpPerspectiveWithParams(circleMaskFrame, &circleMaskFrameW, M,
				image.Pt(frame.Cols(), frame.Rows()), gocv.InterpolationLinear|gocv.WarpInverseMap, gocv.BorderConstant, colorBlack)

			circleMasks[qidx] = circleMaskFrameW
			nbCircleMasks++
		}

		quadrantWindows[qidx].IMShow(resultImg)
	}

	// frameMask := gocv.NewMatWithSize(frame.Rows(), frame.Cols(), gocv.MatTypeCV8U)
	// if nbCircleMasks == 4 {
	// 	gocv.BitwiseOr(circleMasks[0], circleMasks[1], &frameMask)
	// 	gocv.BitwiseOr(frameMask, circleMasks[2], &frameMask)
	// 	gocv.BitwiseOr(frameMask, circleMasks[3], &frameMask)
	// }

	// paint the color dot masks onto the frame
	if nbCircleMasks == 4 {
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

	return isSheetCalibrated
}

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

func prettyPutText(canvas *gocv.Mat, text string, origin image.Point, color color.RGBA, fontScale float64) {
	fontFace := gocv.FontHersheySimplex
	fontThickness := 1
	lineType := gocv.LineAA
	gocv.GetTextSize(text, fontFace, fontScale, fontThickness)
	gocv.PutTextWithParams(canvas, text, origin, fontFace, fontScale, colorBlack, fontThickness+2, lineType, false)
	gocv.PutTextWithParams(canvas, text, origin, fontFace, fontScale, color, fontThickness, lineType, false)
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

	// Create a test point and transform it using M and M_inv

	// return calibrationResults{}

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

	for i := 0; i < 4; i++ {
		deque := NewDeque(5) // Initialize each deque with a capacity
		masks = append(masks, deque)
	}

	cornersWindow := gocv.NewWindow("corners")
	defer cornersWindow.Close()

	quadrantWindows := [4]*gocv.Window{}
	quadrantWindows[0] = gocv.NewWindow("quadrantWindow0")
	defer quadrantWindows[0].Close()
	quadrantWindows[1] = gocv.NewWindow("quadrantWindow1")
	defer quadrantWindows[1].Close()
	quadrantWindows[2] = gocv.NewWindow("quadrantWindow2")
	defer quadrantWindows[2].Close()
	quadrantWindows[3] = gocv.NewWindow("quadrantWindow3")
	defer quadrantWindows[3].Close()

	trackbarsS := debugwindow.CreateTrackbar("Saturation", 255)
	saturation := 100
	trackbarsS.SetPos(saturation)
	trackbarsV := debugwindow.CreateTrackbar("Value", 255)
	value := 75
	trackbarsV.SetPos(value)

	colorHistSums := make([]gocv.Mat, NB_CLRD_CHCKRS)
	nonColorHistSums := make([]gocv.Mat, NB_CLRD_CHCKRS)
	colorModels := make([]gocv.Mat, NB_CLRD_CHCKRS)
	for i := 0; i < NB_CLRD_CHCKRS; i++ {
		colorHistSums[i] = gocv.NewMat()
		nonColorHistSums[i] = gocv.NewMat()
		colorModels[i] = gocv.NewMat()
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
		found := false

		if !isModeled {
			found = gocv.FindChessboardCorners(frameGray, image.Pt(W, H), &corners, gocv.CalibCBAdaptiveThresh+gocv.CalibCBFastCheck)
		}

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
				cornerPointsProj := projectCornerPoints(cornerDotVector, mtx, dist, rvec, tvec)
				sampleColors(frame, cornerPointsProj, colorHistSums, nonColorHistSums, cornersWindow)

				nbHistsSampled++

				if nbHistsSampled > MIN_NB_COLOR_SAMPLES {
					for cidx := 0; cidx < len(colorModels); cidx++ {
						colorSpaceFactor, _ = calcBayesColorModel(colorHistSums[cidx], nonColorHistSums[cidx], &colorModels[cidx])

						fmt.Println("=== Checker colors modeled! ===")
					}

					isModeled = true
				}
			}
		}

		if isModeled && !isSheetCalibrated {
			nbFoundRects, rectangles := predictCheckerColors(frame, &fi.img, cornerColors, colorModels, colorSpaceFactor)
			quadrants := [4][4]image.Point{}

			if nbFoundRects >= 4 {
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

				isSheetCalibrated = calibrateSheet(frame, &fi.img, quadrants, colorHistSums, nonColorHistSums, colorModels, &nbHistsSampled,
					quadrantWindows, trackbarsS, trackbarsV)

			} else {
				fmt.Println("Not enough rectangles found: ", nbFoundRects)
			}
		} else if isSheetCalibrated {
			for pidx, colorModel := range colorModels {
				mask := predictCheckerColor(frame, colorModel, colorSpaceFactor)
				defer mask.Close()

				color := gocv.NewScalar(
					float64(cornerColors[pidx].B),
					float64(cornerColors[pidx].G),
					float64(cornerColors[pidx].R),
					255,
				)

				frameColor := gocv.NewMatWithSize(fi.img.Rows(), fi.img.Cols(), gocv.MatTypeCV8UC3)
				defer frameColor.Close()
				frameColor.SetTo(color)
				frameColor.CopyToWithMask(&fi.img, mask)
			}
		}

		fps := time.Second / time.Since(start)
		exposureTime := getWebcamExposureTime()
		whiteBalanceTemperature := getWebcamwhiteBalanceTemperature()

		prettyPutText(&fi.img, fmt.Sprintf("FPS: %d", fps), image.Pt(10, 15), colorWhite, 0.4)
		prettyPutText(&fi.img, fmt.Sprintf("%s (%d, %d)", fndStr, nbChkBrdFound, nbHistsSampled), image.Pt(10, 30), colorWhite, 0.4)
		prettyPutText(&fi.img, fmt.Sprintf("ExposureTime: %d, WhiteBalanceTemp.: %d", exposureTime, whiteBalanceTemperature), image.Pt(10, 45), colorWhite, 0.4)
		prettyPutText(&fi.img, fmt.Sprintf("isCalibrated: %t, isLocked: %t, isModeled: %t, isSheetCalibrated: %t", isCalibrated, isLocked, isModeled, isSheetCalibrated), image.Pt(10, 60), colorWhite, 0.4)

		if !isCalibrated {
			prettyPutText(&fi.img, "Place the checkerboard", image.Pt(10, 75), colorGreen, 0.3)
		} else if isCalibrated && !isModeled {
			prettyPutText(&fi.img, "Sampling colors ...", image.Pt(10, 75), colorGreen, 0.3)
		} else if isModeled && !isSheetCalibrated {
			prettyPutText(&fi.img, "Hold the calibration sheet in the middle", image.Pt(10, 75), colorGreen, 0.3)
			prettyPutText(&fi.img, "and align each calibration color to each quadrant color", image.Pt(10, 85), colorGreen, 0.3)
		}

		if found && !isCalibrated && nbChkBrdFound >= MIN_NB_CHKBRD_FOUND {
			reproj_err := gocv.CalibrateCamera(objectPoints, imgPoints, image.Pt(W, H), &mtx, &dist, &rvecs, &tvecs, 0)

			fmt.Println("=== Calibrated! === Reprojection error:", reproj_err)
			isCalibrated = true

			if !isLocked {
				lockWebcam(exposureTime, whiteBalanceTemperature)
				isLocked = true
			}
		}

		if !rvec.Empty() {
			// Draw the axes
			drawAxes(&fi.img, axesVector, mtx, dist, rvec, tvec)

		} else {
			// Draw and display the corners
			gocv.DrawChessboardCorners(&fi.img, image.Pt(W, H), corners, found)
		}

		fi.debugWindow.IMShow(fi.img)
		fi.projection.IMShow(fi.cimg)
		key := fi.debugWindow.WaitKey(WAIT)
		if key == 27 {
			cornersWindow.Close()
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

	return calibrationResults{
		rvec: MatToFloat32Slice(rvecs),
		tvec: MatToFloat32Slice(tvecs),
		mtx:  mtx,
		dist: dist,
	}
}
