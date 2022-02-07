all: build/fdbsync

rebuild: clean build/fdbsync

clean:
	rm -rf bin/*

build/fdbsync:
	CGO_ENABLED=0 go build -o build/fdbsync