GOFMT_FILES?=$$(find . -name '*.go')

default: all

all: fmt ecstool

fmt:
	gofmt -w $(GOFMT_FILES)

ecstool:
	go build ecstool.go

clean:
	rm -f ecstool

install: ecstool
	go install 
