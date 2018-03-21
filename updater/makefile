APP = updater
SOURCE = cmd/updater.go

all: build zip clean

build:
	env GOOS=linux GOARCH=amd64 go build $(SOURCE)

zip:
	zip -r $(APP).zip $(APP)

clean:
	rm $(APP)