.PHONY: help

GCP_PROJECT ?= piazzaapp
APP_NAME ?= forge-cli
APP_VSN ?= `cat VERSION`
BUILD ?= `git rev-parse --short HEAD`
DKR_HOST ?= dkr.piazza.app

help:
	@perl -nle'print $& if m{^[a-zA-Z_-]+:.*?## .*$$}' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

install:
	go install

release:
	GOOS=linux go build -o build/forge.linux
	GOOS=darwin go build -o build/forge.darwin
	GOOS=windows go build -o build/forge.exe

build: ## Build the Docker image
	docker build --build-arg APP_NAME=$(APP_NAME) \
		--build-arg APP_VSN=$(APP_VSN) \
		-t $(APP_NAME):$(APP_VSN) \
		-t $(APP_NAME):latest \
		-t gcr.io/$(GCP_PROJECT)/$(APP_NAME):$(APP_VSN) \
		-t $(DKR_HOST)/forge/$(APP_NAME):$(APP_VSN) .

push: ## push to gcr
	docker push gcr.io/$(GCP_PROJECT)/$(APP_NAME):$(APP_VSN)
	docker push $(DKR_HOST)/forge/${APP_NAME}:$(APP_VSN)

generate:
	go generate ./...