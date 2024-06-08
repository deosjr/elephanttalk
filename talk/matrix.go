package talk

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"gocv.io/x/gocv"
)

const EPSILON = 0.0000001

func matToDoubleSlice64F(mat gocv.Mat) []float64 {
	if !(mat.Type() == gocv.MatTypeCV64F || mat.Type() == gocv.MatTypeCV64FC3) {
		fmt.Println("matToDoubleSlice64F mat type", mat.Type())
		return nil
	}
	data, _ := mat.DataPtrFloat64()
	return data
}
func matsToDoubleSlice64F(mats []gocv.Mat) [][]float64 {
	var data [][]float64
	for _, mat := range mats {
		data = append(data, matToDoubleSlice64F(mat))
	}
	return data
}

func matToFloatSlice32F(mat gocv.Mat) []float32 {
	if !(mat.Type() == gocv.MatTypeCV32F || mat.Type() == gocv.MatTypeCV32FC3) {
		fmt.Println("matToFloatSlice32F mat type", mat.Type())
		return nil
	}
	data, _ := mat.DataPtrFloat32()
	return data
}

func matsToFloatSlice32F(mats []gocv.Mat) [][]float32 {
	var data [][]float32
	for _, mat := range mats {
		data = append(data, matToFloatSlice32F(mat))
	}
	return data
}

func matToFloatSlice16S(mat gocv.Mat) []int32 {
	var data []int32
	rows := mat.Rows()
	cols := mat.Cols()
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			value := mat.GetIntAt(r, c)
			data = append(data, value)
		}
	}
	return data
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

func PrintMatValues32F(mat gocv.Mat) {
	// Assume mat is a 3D matrix where each point is a single float
	rows := mat.Rows()
	cols := mat.Cols()

	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			val := mat.GetFloatAt(r, c)
			fmt.Printf("%9.8f ", val)
		}
		fmt.Println() // New line for each row
	}
}

func PrintMatValues32FC(mat gocv.Mat) {
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

func matToFloatSlice32FC(mat gocv.Mat) []float32 {
	if mat.Type() != gocv.MatTypeCV32F {
		fmt.Println("mat type", mat.Type())
		return nil
	}
	data, _ := mat.DataPtrFloat32()
	return data
}

func matsToFloatSlice32FC(mats []gocv.Mat) [][]float32 {
	var data [][]float32
	for _, mat := range mats {
		data = append(data, matToFloatSlice32FC(mat))
	}
	return data
}

func doubleSliceToMat64F(data []float64, rows, cols, channels int) (gocv.Mat, error) {
	data_bytes := make([]byte, len(data)*8)
	for i := 0; i < len(data); i++ {
		bits := *(*uint64)(unsafe.Pointer(&data[i]))
		binary.LittleEndian.PutUint64(data_bytes[i*8:i*8+8], bits)
	}
	if channels == 1 {
		return gocv.NewMatFromBytes(rows, cols, gocv.MatTypeCV64F, data_bytes)
	} else if channels == 3 {
		return gocv.NewMatFromBytes(rows, cols, gocv.MatTypeCV64FC3, data_bytes)
	}
	return gocv.NewMat(), fmt.Errorf("invalid number of channels")
}

func doubleSliceToNDMat64F(data []float64, sizes []int) (gocv.Mat, error) {
	data_bytes := make([]byte, len(data)*8)
	for i := 0; i < len(data); i++ {
		bits := *(*uint64)(unsafe.Pointer(&data[i]))
		binary.LittleEndian.PutUint64(data_bytes[i*8:i*8+8], bits)
	}
	return gocv.NewMatWithSizesFromBytes(sizes, gocv.MatTypeCV64F, data_bytes)

}

func floatSliceToMat32F(data []float64, rows, cols, channels int) (gocv.Mat, error) {
	mat, err := doubleSliceToMat64F(data, rows, cols, channels)
	if err != nil {
		fmt.Println("Error creating mat")
		return gocv.NewMat(), err
	}
	mat.ConvertTo(&mat, gocv.MatTypeCV32F)
	return mat, err
}

func floatToMats32F(data [][]float64, sizes []int, nbMats int) ([]gocv.Mat, error) {
	mats := make([]gocv.Mat, nbMats)
	for i := 0; i < nbMats; i++ {
		mat, err := doubleSliceToNDMat64F(data[i], sizes)
		if err == nil {
			mats[i] = gocv.NewMat()
			mat.ConvertTo(&mats[i], gocv.MatTypeCV32F)
		} else {
			fmt.Println("Error creating mat", i)
			errMsg := fmt.Errorf("error creating mat %d", i)
			return nil, errMsg
		}
	}
	return mats, nil
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
