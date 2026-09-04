FROM golang:alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy only the necessary source code directories for the build
COPY cmd/ ./cmd/
COPY internal/ ./internal/

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

# Declare /app/cache as a mount point so the holiday cache persists across restarts
VOLUME ["/app/cache"]

# Command to run the application
CMD ["./byeclocking"]
