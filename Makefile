.PHONY: build run clean

BINARY_NAME=yt-tui

build:
	go build -o $(BINARY_NAME) main.go

run: build
	./$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)
