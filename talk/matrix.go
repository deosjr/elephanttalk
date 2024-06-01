package talk

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"gocv.io/x/gocv"
)

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
