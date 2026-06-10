# --- Build Stage ---
FROM golang:1.25-alpine AS builder

# Install system dependencies (git for downloading modules if needed)
RUN apk add --no-cache git

# Set working directory inside container
WORKDIR /app

# Copy dependency files and fetch modules
COPY go.mod ./
RUN go mod download

# Copy the rest of the application code
COPY . .

# Compile a statically linked Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /hotel-server ./cmd/server/main.go

# --- Final Runner Stage ---
FROM alpine:3.20

# Install base runtime utilities (ca-certificates and timezone data)
RUN apk add --no-cache ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /hotel-server ./hotel-server

# Copy database migrations/seed scripts
COPY --from=builder /app/db ./db



# Set production environment flags
ENV GIN_MODE=release
ENV PORT=8080

# Expose server port
EXPOSE 8080

# Execute server binary
CMD ["./hotel-server"]
