# terraform-provider-paperclip/Makefile
BINARY=terraform-provider-paperclip
build:
	go build -o $(BINARY) .
test:
	go test ./... -count=1
testacc:
	TF_ACC=1 go test ./internal/provider -count=1 -v -timeout 20m
install: build
	mkdir -p $(HOME)/go/bin && cp $(BINARY) $(HOME)/go/bin/$(BINARY)
