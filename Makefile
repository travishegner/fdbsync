all: build/fdbsync

rebuild: clean build/fdbsync

clean:
	rm -rf build/*

build/fdbsync: lint
	CGO_ENABLED=0 go build -o build/fdbsync

lint:
	golint -set_exit_status ./...

.PHONY: all rebuild clean lint