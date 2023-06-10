run:
	go run cmd/elephanttalk/main.go

test:
	go test ./... -test.short -count=1

clean:
	rm -rf build

build: clean
	go build -o ./build/elephanttalk ./cmd/elephanttalk/

install:
	go install ./cmd/elephanttalk
