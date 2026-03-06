# build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# install dependencies
COPY go.mod go.sum ./
RUN go mod download

# copy source
COPY . .

# build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# run stage
FROM alpine:3.21

WORKDIR /app

# install ca-certificates for HTTPS and wget for healthcheck
RUN apk --no-cache add ca-certificates wget

# copy binary from builder
COPY --from=builder /app/server .

# create uploads directory
RUN mkdir -p uploads

EXPOSE 8080

CMD ["./server"]