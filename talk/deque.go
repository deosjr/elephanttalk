package talk

import (
	"gocv.io/x/gocv"
)

type Deque struct {
	elements []*gocv.Mat
	capacity int
}

func NewDeque(capacity int) *Deque {
	return &Deque{
		elements: make([]*gocv.Mat, 0),
		capacity: capacity,
	}
}

func (d *Deque) Push(mat *gocv.Mat) {
	if len(d.elements) >= d.capacity {
		d.Pop() // Automatically remove and close the oldest mat
	}
	d.elements = append(d.elements, mat)
}

func (d *Deque) Pop() *gocv.Mat {
	if len(d.elements) == 0 {
		return nil
	}
	oldest := d.elements[0]
	d.elements = d.elements[1:]
	oldest.Close() // Close the oldest mat's memory
	return oldest
}

func (d *Deque) Clear() {
	for _, mat := range d.elements {
		mat.Close() // Close each mat
	}
	d.elements = nil
}

func (d *Deque) Iter() <-chan *gocv.Mat {
	ch := make(chan *gocv.Mat)
	go func() {
		for _, mat := range d.elements {
			ch <- mat
		}
		close(ch)
	}()
	return ch
}

// Size returns the number of elements in the deque
func (d *Deque) Size() int {
	return len(d.elements)
}

func (d *Deque) Empty() bool {
	return d.Size() == 0
}
