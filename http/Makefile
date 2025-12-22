.PHONY: swagger run

swagger:
	@echo "Generating swagger documentation..."
	swag init -g cmd/main.go

run: swagger
	@echo "Starting application..."
	go run cmd/main.go

swagger-watch:
	@echo "Watching and regenerating swagger..."
	@while true; do \
		swag init -g cmd/main.go; \
		sleep 2; \
	done
	