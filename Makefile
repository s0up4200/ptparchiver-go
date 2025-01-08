VERSION ?= dev
COMMIT = $(shell git rev-parse --short HEAD)
DATE = $(shell git log -1 --format=%ct)

build:
	go build -ldflags "\
		-X main.appVersion=$(VERSION) \
		-X main.appCommit=$(COMMIT) \
		-X main.appDate=$(DATE)" \
		-o ptparchiver ./cmd/ptparchiver
