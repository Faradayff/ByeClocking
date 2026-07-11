FROM golang:alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application statically
# CGO_ENABLED=0 ensures a static binary, crucial for minimal containers
RUN CGO_ENABLED=0 GOOS=linux go build -o byeclocking ./cmd/byeclocking

# Final stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates (for HTTPS) and tzdata (for timezone support)
RUN apk --no-cache add ca-certificates tzdata

# Copy the built binary from the builder stage
COPY --from=builder /app/byeclocking .

# Command to run the application
CMD ["./byeclocking"]
