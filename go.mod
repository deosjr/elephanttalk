module github.com/deosjr/elephanttalk

go 1.21

toolchain go1.22.2

require (
	github.com/deosjr/whistle v0.0.0-20230606141022-90a4546b49c5
	gocv.io/x/gocv v0.36.1-chessboard
)

require gonum.org/v1/gonum v0.15.0 // indirect

// replace gocv.io/x/gocv => ../../coert/gocv
replace gocv.io/x/gocv => ../gocv
