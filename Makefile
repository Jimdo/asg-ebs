default: build

build:
	docker run -v $(CURDIR):/src centurylink/golang-builder

