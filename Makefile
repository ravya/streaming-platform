.PHONY: test run token
test:
	go vet ./... && go test -race -cover ./...
run:
	JWT_SECRET=dev-secret go run ./cmd/insights-api
token:
	JWT_SECRET=dev-secret go run ./cmd/tokengen $(TENANT)
