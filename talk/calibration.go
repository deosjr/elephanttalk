package talk

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"gocv.io/x/gocv"
)

type calibrationResults struct {
	pixelsPerCM     float64
	displacement    point
	displayRatio    float64
	referenceColors []color.RGBA
}

func MatToFloat32Slice(mat gocv.Mat) [][]float32 {
	if mat.Empty() {
		return nil
	}

	// Get the total number of elements
	mtxType := mat.Type()
	channels := mat.Channels()
	totalElements := mat.Total()
	fmt.Println("mtxType:", mtxType, "channels:", channels, "totalElements:", totalElements)

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

	return data
}

func chessBoardCalibration(webcam *gocv.VideoCapture, debugwindow, projection *gocv.Window) calibrationResults {
	w := 13
	h := 6
	CHESS_WIDTH := float32(w)
	CHESS_HEIGHT := float32(h)
	nbFound := 0
	minNbFound := 50
	isCalibrated := false
	cw := 100
	ch := 100
	cb := 15

	cornerDots := [][]float32{
		{0, 0, 0}, // red LT
		{0, -1, 0},
		{-1, -1, 0},
		{-1, 0, 0},
		{CHESS_WIDTH, 0, 0}, // purple RT
		{CHESS_WIDTH, -1, 0},
		{CHESS_WIDTH - 1, -1, 0},
		{CHESS_WIDTH - 1, 0, 0},
		{0, CHESS_HEIGHT, 0}, // orange LB
		{0, CHESS_HEIGHT - 1, 0},
		{-1, CHESS_HEIGHT - 1, 0},
		{-1, CHESS_HEIGHT, 0},
		{CHESS_WIDTH, CHESS_HEIGHT, 0}, // green RB
		{CHESS_WIDTH, CHESS_HEIGHT - 1, 0},
		{CHESS_WIDTH - 1, CHESS_HEIGHT - 1, 0},
		{CHESS_WIDTH - 1, CHESS_HEIGHT, 0},
	}
	cornerDotVector := gocv.NewPoint3fVector()
	defer cornerDotVector.Close()
	for i := 0; i < len(cornerDots); i++ {
		point := gocv.NewPoint3f(cornerDots[i][0], cornerDots[i][1], cornerDots[i][2])
		cornerDotVector.Append(point)
	}

	// prepare object points, like (0,0,0), (1,0,0), (2,0,0) ....,(6,5,0)
	objectPoints := gocv.NewPoints3fVector()
	defer objectPoints.Close()
	imgPoints := gocv.NewPoints2fVector()
	defer imgPoints.Close()

	// gocv.NewPoint3fVectorFromPoints([]gocv.Point3f{})

	objp := make([][]float32, w*h)
	for i := range objp {
		objp[i] = make([]float32, 3)
	}

	p3fv := gocv.NewPoint3fVector()
	defer p3fv.Close()
	for i := 0; i < h; i++ {

		for j := 0; j < w; j++ {
			objp[i*w+j][0] = float32(j)
			objp[i*w+j][1] = float32(i)
			objp[i*w+j][2] = float32(0)

			point := gocv.NewPoint3f(objp[i*w+j][0], objp[i*w+j][1], objp[i*w+j][2])
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
	waitMillis := 100

	cornerColors := []color.RGBA{
		{13, 158, 56, 0},  // green
		{22, 140, 250, 0}, // orange
		{133, 16, 57, 0},  // purple
		{45, 34, 245, 0},  // red
	}

	mtx := gocv.NewMat()
	defer mtx.Close()
	dist := gocv.NewMat()
	defer dist.Close()

	corners_imshow := gocv.NewWindow("corners")
	defer corners_imshow.Close()

	for {
		start := time.Now()
		if ok := fi.webcam.Read(&fi.img); !ok {
			break
		}
		if fi.img.Empty() {
			continue
		}

		gray_img := gocv.NewMat()
		defer gray_img.Close()

		// convert the rgb frame into gray
		gocv.CvtColor(fi.img, &gray_img, gocv.ColorBGRToGray)

		// Find the chess board corners
		corners := gocv.NewMat()
		defer corners.Close()
		found := gocv.FindChessboardCorners(gray_img, image.Pt(w, h), &corners, gocv.CalibCBAdaptiveThresh+gocv.CalibCBFastCheck)
		fndStr := ""
		if found {
			fndStr = "FOUND"
		} else {
			fndStr = "NOT FOUND"
		}

		if found && !isCalibrated {
			objectPoints.Append(p3fv)

			gocv.CornerSubPix(gray_img, &corners, image.Pt(11, 11), image.Pt(-1, -1), termCriteria)
			point2fVector := gocv.NewPoint2fVectorFromMat(corners)
			defer point2fVector.Close()
			imgPoints.Append(point2fVector)

			nbFound++
		}

		if found && isCalibrated {
			gocv.CornerSubPix(gray_img, &corners, image.Pt(11, 11), image.Pt(-1, -1), termCriteria)

			point2fVector := gocv.NewPoint2fVectorFromMat(corners)
			defer point2fVector.Close()
			rvec := gocv.NewMat()
			defer rvec.Close()
			tvec := gocv.NewMat()
			defer tvec.Close()
			inliers := gocv.NewMat()
			defer inliers.Close()

			gocv.SolvePnPRansac(p3fv, point2fVector, mtx, dist, &rvec, &tvec, false, 100, 8, 0.99, &inliers, 0)

			projPoints := gocv.NewPoint2fVector()
			defer projPoints.Close()
			jacobian := gocv.NewMat()
			defer jacobian.Close()

			gocv.ProjectPoints(cornerDotVector, rvec, tvec, mtx, dist, projPoints, &jacobian, 0)

			for i := 0; i < projPoints.Size(); i++ {
				point := projPoints.At(i)
				image_point := image.Pt(int(point.X), int(point.Y))
				gocv.Circle(&fi.cimg, image_point, 4, color.RGBA{255, 255, 255, 0}, 2)
			}

			// corner_points will store the results
			var corner_points []([]interface{})

			for pidx := 0; pidx < projPoints.Size()-3; pidx += 4 {
				proj_pt1 := []int{int(projPoints.At(pidx).X), int(projPoints.At(pidx).Y)}
				proj_pt2 := []int{int(projPoints.At(pidx + 1).X), int(projPoints.At(pidx + 1).Y)}
				proj_pt3 := []int{int(projPoints.At(pidx + 2).X), int(projPoints.At(pidx + 2).Y)}
				proj_pt4 := []int{int(projPoints.At(pidx + 3).X), int(projPoints.At(pidx + 3).Y)}

				corner_points = append(corner_points, []interface{}{
					[]([]int){proj_pt1, proj_pt2, proj_pt3, proj_pt4},
					cornerColors[pidx/4],
				})
			}

			final_crops := gocv.NewMat()
			defer final_crops.Close()

			// Loop over corner_points and convert points to gocv.PointsVector
			for _, corner_point := range corner_points {
				points := corner_point[0].([][]int)
				// clr := corner_point[1].(string)

				// Create a gocv.PointsVector from points
				pointsVector := gocv.NewPointsVector()
				defer pointsVector.Close()

				// Convert points to gocv.Point
				image_points := gocv.NewPointVector()
				defer image_points.Close()
				projPointsArr := []gocv.Point2f{}
				for _, pt := range points {
					image_point := image.Pt(pt[0], pt[1])
					image_points.Append(image_point)

					point2f := gocv.Point2f{X: float32(image_point.X), Y: float32(image_point.Y)}
					projPointsArr = append(projPointsArr, point2f)
				}
				pointsVector.Append(image_points)

				// Create a mask for the polygon
				mask := gocv.NewMatWithSize(fi.img.Rows(), fi.img.Cols(), gocv.MatTypeCV8U)
				defer mask.Close()
				gocv.FillPoly(&mask, pointsVector, color.RGBA{255, 255, 255, 255})

				// Invert the mask
				maskInv := gocv.NewMat()
				defer maskInv.Close()
				gocv.BitwiseNot(mask, &maskInv)

				// Extract the poly from the image using the mask
				poly := gocv.NewMat()
				defer poly.Close()
				gocv.BitwiseAndWithMask(fi.img, fi.img, &poly, mask)

				cornerDots := [][]int{{0, 0}, {cw, 0}, {cw, ch}, {0, ch}}
				p2fs := []gocv.Point2f{}
				for i := 0; i < len(cornerDots); i++ {
					point := image.Pt(cornerDots[i][0], cornerDots[i][1])
					p2fs = append(p2fs, gocv.Point2f{X: float32(point.X), Y: float32(point.Y)})
				}

				pafv := gocv.NewPoint2fVectorFromPoints(projPointsArr)
				defer pafv.Close()
				p2fv := gocv.NewPoint2fVectorFromPoints(p2fs)
				defer p2fv.Close()

				M := gocv.GetPerspectiveTransform2f(pafv, p2fv)
				defer M.Close()
				tPoly := gocv.NewMat()
				defer tPoly.Close()
				gocv.WarpPerspective(poly, &tPoly, M, image.Pt(cw, ch))

				// Create the rectangle for the region of interest
				rect := image.Rect(cb, cb, cw-cb, ch-cb)
				// Crop the transformed_polygon
				final_crop := tPoly.Region(rect)
				defer final_crop.Close()

				if final_crops.Empty() {
					final_crops = final_crop
				} else {
					gocv.Vconcat(final_crops, final_crop, &final_crops)
				}
			}

			corners_imshow.IMShow(final_crops)
		}

		gocv.PutText(&fi.img, fmt.Sprintf("%s (%d)", fndStr, nbFound), image.Pt(0, 40), 0, .5, color.RGBA{255, 0, 0, 0}, 2)

		if found && !isCalibrated && nbFound >= minNbFound {
			rvecs := gocv.NewMat()
			defer rvecs.Close()
			tvecs := gocv.NewMat()
			defer tvecs.Close()
			reproj_err := gocv.CalibrateCamera(objectPoints, imgPoints, image.Pt(w, h), &mtx, &dist, &rvecs, &tvecs, 0)
			fmt.Println("=== Calibrated! === Reprojection error:", reproj_err)

			rvecs_f32 := MatToFloat32Slice(rvecs)
			fmt.Println("rvecs_f32", rvecs_f32)
			tvecs_f32 := MatToFloat32Slice(tvecs)
			fmt.Println("tvecs_f32", tvecs_f32)
			mtx_f32 := MatToFloat32Slice(mtx)
			fmt.Println("mtx_f32", mtx_f32)
			dist_f32 := MatToFloat32Slice(dist)
			fmt.Println("dist_f32", dist_f32)

			isCalibrated = true
		}

		// Draw and display the corners
		gocv.DrawChessboardCorners(&fi.img, image.Pt(w, h), corners, found)

		fps := time.Second / time.Since(start)
		gocv.PutText(&fi.img, fmt.Sprintf("FPS: %d", fps), image.Pt(0, 20), 0, .5, color.RGBA{0, 255, 0, 0}, 2)

		fi.debugWindow.IMShow(fi.img)
		fi.projection.IMShow(fi.cimg)
		key := fi.debugWindow.WaitKey(waitMillis)
		if key >= 0 {
			break
		}
	}

	pixPerCM := 0 / 1.0
	displacement := point{0, 0}
	displayRatio := 1.0
	colorSamples := []color.RGBA{}
	return calibrationResults{pixPerCM, displacement, displayRatio, colorSamples}
}

// func calibration(webcam *gocv.VideoCapture, debugwindow, projection *gocv.Window) calibrationResults {
// 	//calibrationPage()

// 	// Step 1: set up beamer + webcam at a surface
// 	// Step 2: drag projector window to the beamer
// 	// Step 3: beamer projects midpoint -> position calibration pattern centered on midpoint
// 	// Step 4: recognise pattern and calculate pixel distances

// 	// TODO probably rename these
// 	// img: debug window output from camera
// 	// cimg: projector window
// 	img := gocv.NewMat()
// 	defer img.Close()

// 	cimg := gocv.NewMatWithSize(beamerHeight, beamerWidth, gocv.MatTypeCV8UC3)
// 	defer cimg.Close()
// 	red := color.RGBA{255, 0, 0, 0}
// 	green := color.RGBA{0, 255, 0, 0}
// 	blue := color.RGBA{0, 0, 255, 0}
// 	yellow := color.RGBA{255, 255, 0, 0}

// 	w, h := beamerWidth/2., beamerHeight/2.
// 	gocv.Line(&cimg, image.Pt(w-5, h), image.Pt(w+5, h), red, 2)
// 	gocv.Line(&cimg, image.Pt(w, h-5), image.Pt(w, h+5), red, 2)
// 	gocv.PutText(&cimg, "Place calibration pattern", image.Pt(w-100, h+50), 0, .5, color.RGBA{255, 255, 255, 0}, 2)

// 	var pattern []circle

// 	fi := frameInput{
// 		webcam:      webcam,
// 		debugWindow: debugwindow,
// 		projection:  projection,
// 		img:         img,
// 		cimg:        cimg,
// 	}

// 	if err := frameloop(fi, func(_ image.Image, spatialPartition map[image.Rectangle][]circle) {
// 		// find calibration pattern, draw around it
// 		for k, v := range spatialPartition {
// 			if !findCalibrationPattern(v) {
// 				continue
// 			}
// 			sortCirclesAsCorners(v)
// 			gocv.Rectangle(&img, k, red, 2)
// 			r := image.Rectangle{
// 				v[0].mid.add(point{-v[0].r, -v[0].r}).toIntPt(),
// 				v[3].mid.add(point{v[3].r, v[3].r}).toIntPt(),
// 			}
// 			gocv.Rectangle(&img, r, blue, 2)

// 			// TODO: draw indicators for horizontal/vertical align
// 			pattern = v
// 		}

// 	}, nil, 100); err != nil {
// 		return calibrationResults{}
// 	}

// 	// keypress breaks the loop, assume pattern is found over midpoint
// 	// draw conclusions about colors and distances
// 	webcamMid := circlesMidpoint(pattern)
// 	// average over all 4 distances to circles in pattern
// 	dpixels := euclidian(pattern[0].mid.sub(webcamMid))
// 	dpixels += euclidian(pattern[1].mid.sub(webcamMid))
// 	dpixels += euclidian(pattern[2].mid.sub(webcamMid))
// 	dpixels += euclidian(pattern[3].mid.sub(webcamMid))
// 	dpixels = dpixels / 4.
// 	dcm := math.Sqrt(1.5*1.5 + 1.5*1.5)

// 	// just like for printing 1cm = 118px, we need a new ratio for projections
// 	// NOTE: pixPerCM lives in webcamspace, NOT beamerspace
// 	pixPerCM := dpixels / dcm

// 	// beamer midpoint vs webcam midpoint displacement
// 	beamerMid := point{float64(w), float64(h)}
// 	displacement := beamerMid.sub(webcamMid)

// 	// get color samples of the four dots as reference values
// 	var colorSamples []color.RGBA
// 	for {
// 		if ok := webcam.Read(&img); !ok {
// 			fmt.Printf("cannot read device\n")
// 			return calibrationResults{}
// 		}
// 		if img.Empty() {
// 			continue
// 		}

// 		actualImage, _ := img.ToImage()
// 		if colorSamples == nil {
// 			// TODO: average within the circle?
// 			colorSamples = make([]color.RGBA, 4)
// 			for i, circle := range pattern {
// 				c := actualImage.At(int(circle.mid.x), int(circle.mid.y))
// 				rr, gg, bb, _ := c.RGBA()
// 				colorSamples[i] = color.RGBA{uint8(rr), uint8(gg), uint8(bb), 0}
// 			}
// 		}
// 		break
// 	}

// 	// project another reference point and calculate diff between webcam-space and projector-space
// 	// ratio between webcam and beamer
// 	displayRatio := 1.0

// 	if err := frameloop(fi, func(_ image.Image, spatialPartition map[image.Rectangle][]circle) {
// 		gocv.Rectangle(&cimg, image.Rect(0, 0, beamerWidth, beamerHeight), color.RGBA{}, -1)
// 		gocv.Line(&cimg, image.Pt(w-5+200, h), image.Pt(w+5+200, h), red, 2)
// 		gocv.Line(&cimg, image.Pt(w+200., h-5), image.Pt(w+200, h+5), red, 2)
// 		gocv.PutText(&cimg, "Place calibration pattern", image.Pt(w-100+200, h+50), 0, .5, color.RGBA{255, 255, 255, 0}, 2)

// 		// find calibration pattern, draw around it
// 		for k, v := range spatialPartition {
// 			if !findCalibrationPattern(v) {
// 				continue
// 			}
// 			sortCirclesAsCorners(v)
// 			gocv.Rectangle(&img, k, red, 2)
// 			r := image.Rectangle{
// 				v[0].mid.add(point{-v[0].r, -v[0].r}).toIntPt(),
// 				v[3].mid.add(point{v[3].r, v[3].r}).toIntPt(),
// 			}
// 			gocv.Rectangle(&img, r, blue, 2)

// 			midpoint := circlesMidpoint(v)
// 			// assume Y component stays 0 (i.e. we are horizontally aligned between webcam and beamer)
// 			displayRatio = midpoint.sub(webcamMid).x / 200.0

// 			// projecting the draw ratio difference
// 			withoutRatio := midpoint.add(displacement).toIntPt()
// 			gocv.Line(&cimg, beamerMid.toIntPt(), withoutRatio, blue, 2)

// 			// TODO: draw indicators for horizontal/vertical align
// 			pattern = v
// 		}

// 		gocv.Circle(&img, image.Pt(10, 10), 10, colorSamples[0], -1)
// 		gocv.Circle(&img, image.Pt(30, 10), 10, colorSamples[1], -1)
// 		gocv.Circle(&img, image.Pt(50, 10), 10, colorSamples[2], -1)
// 		gocv.Circle(&img, image.Pt(70, 10), 10, colorSamples[3], -1)

// 	}, colorSamples, 100); err != nil {
// 		return calibrationResults{}
// 	}

// 	if err := frameloop(fi, func(actualImage image.Image, spatialPartition map[image.Rectangle][]circle) {
// 		gocv.Rectangle(&cimg, image.Rect(0, 0, beamerWidth, beamerHeight), color.RGBA{}, -1)

// 		for k, v := range spatialPartition {
// 			if !findCalibrationPattern(v) {
// 				continue
// 			}
// 			sortCirclesAsCorners(v)

// 			colorDiff := make([]float64, 4)
// 			for i, circle := range v {
// 				c := actualImage.At(int(circle.mid.x), int(circle.mid.y))
// 				colorDiff[i] = colorDistance(c, colorSamples[i])
// 			}
// 			// experimentally, all diffs under 10k means we are good (paper rightway up)
// 			// unless ofc lighting changes drastically

// 			gocv.Rectangle(&img, k, red, 2)
// 			r := image.Rectangle{
// 				v[0].mid.add(point{-v[0].r, -v[0].r}).toIntPt(),
// 				v[3].mid.add(point{v[3].r, v[3].r}).toIntPt(),
// 			}
// 			gocv.Rectangle(&img, r, blue, 2)

// 			gocv.Circle(&img, v[0].mid.toIntPt(), int(v[0].r), red, 2)
// 			gocv.Circle(&img, v[1].mid.toIntPt(), int(v[1].r), green, 2)
// 			gocv.Circle(&img, v[2].mid.toIntPt(), int(v[2].r), blue, 2)
// 			gocv.Circle(&img, v[3].mid.toIntPt(), int(v[3].r), yellow, 2)

// 			// now we project around the whole A4 containing the calibration pattern
// 			// a4 in cm: 21 x 29.7
// 			a4hpx := (29.7 * pixPerCM) / 2.
// 			a4wpx := (21.0 * pixPerCM) / 2.
// 			midpoint := circlesMidpoint(v)
// 			min := midpoint.add(point{-a4wpx, -a4hpx})
// 			max := midpoint.add(point{a4wpx, a4hpx})
// 			a4 := image.Rectangle{min.toIntPt(), max.toIntPt()}
// 			gocv.Rectangle(&img, a4, blue, 4)

// 			// adjust for displacement and display ratio
// 			a4 = image.Rectangle{translate(min, displacement, displayRatio).toIntPt(), translate(max, displacement, displayRatio).toIntPt()}
// 			gocv.Rectangle(&cimg, a4, blue, 4)
// 		}

// 		gocv.Circle(&img, image.Pt(10, 10), 10, colorSamples[0], -1)
// 		gocv.Circle(&img, image.Pt(30, 10), 10, colorSamples[1], -1)
// 		gocv.Circle(&img, image.Pt(50, 10), 10, colorSamples[2], -1)
// 		gocv.Circle(&img, image.Pt(70, 10), 10, colorSamples[3], -1)
// 	}, colorSamples, 100); err != nil {
// 		return calibrationResults{}
// 	}

// 	// TODO: happy (y/n) ? if no return to start of calibration
// 	return calibrationResults{pixPerCM, displacement, displayRatio, colorSamples}
// }
