package talk

import (
	"image"

	"gocv.io/x/gocv"
)

func findCornerDots(mask gocv.Mat) gocv.Mat {
	hullsPointsVector := gocv.NewPointsVector()
	defer hullsPointsVector.Close()
	checkerHullPointsVector := gocv.NewPointsVector()
	defer checkerHullPointsVector.Close()
	contoursVector := gocv.FindContours(mask, gocv.RetrievalList, gocv.ChainApproxSimple)
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

		hullPointVector := gocv.NewPointVectorFromPoints(hullPoints)
		defer hullPointVector.Close()
		hullPointsVector := gocv.NewPointsVector()
		defer hullPointsVector.Close()
		hullPointsVector.Append(hullPointVector)

		contourMask := gocv.NewMatWithSize(mask.Rows(), mask.Cols(), gocv.MatTypeCV8U)
		defer contourMask.Close()
		gocv.FillPoly(&contourMask, hullPointsVector, colorWhite)

		center := getCenter(hullPointVector.ToPoints())
		if contourMask.GetUCharAt(center.Y, center.X) == 255 {
			dist := gocv.NewMat()
			defer dist.Close()
			labels := gocv.NewMat()
			defer labels.Close()
			gocv.DistanceTransform(contourMask, &dist, &labels, gocv.DistL2, 3, gocv.DistanceLabelCComp)

			// distToEdge := dist.GetFloatAt(center.Y, center.X)
			// fmt.Println("distToEdge", distToEdge)
		}

		hullsPointsVector.Append(hullPointVector)
	}

	contourMask := gocv.NewMatWithSize(mask.Rows(), mask.Cols(), gocv.MatTypeCV8U)
	defer contourMask.Close()
	gocv.FillPoly(&contourMask, hullsPointsVector, colorWhite)

	gocv.NewWindow("contourMask").IMShow(contourMask)

	return mask
}
