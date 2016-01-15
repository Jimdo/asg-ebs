default: build

build:
	docker run -v $(CURDIR):/src centurylink/golang-builder

release:
	docker run -v $(CURDIR):/src -e CGO_ENABLED=true -e LDFLAGS='-extldflags "-static"' -e COMPRESS_BINARY=true centurylink/golang-builder

