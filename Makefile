default: build

guard-%:
	@ if [ "${${*}}" = "" ]; then \
		echo "Environment variable $* not set"; \
		exit 1; \
	fi

test:
	docker run -v $(CURDIR):/src centurylink/golang-tester 

build:
	docker run -v $(CURDIR):/src centurylink/golang-builder

release-build:
	docker run -v $(CURDIR):/src -e CGO_ENABLED=true -e LDFLAGS='-extldflags "-static"' -e COMPRESS_BINARY=true centurylink/golang-builder

release: release-build guard-GITHUB_TOKEN guard-VERSION
	git tag v$(VERSION) && git push origin v$(VERSION)
	docker run -e GITHUB_TOKEN=$(GITHUB_TOKEN) jimdo/github-release release --user Jimdo --repo asg-ebs --tag v$(VERSION)
	docker run -e GITHUB_TOKEN=$(GITHUB_TOKEN) -v $(CURDIR):/src -w /src jimdo/github-release upload --user Jimdo --repo asg-ebs --tag v$(VERSION) --name "asg-ebs" --file asg-ebs
