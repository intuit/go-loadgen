# Build a static binary with a supported Go release...
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /loadgen .

# ...and copy it into a minimal base image.
FROM gcr.io/distroless/static-debian13:nonroot
LABEL maintainer="chaitanya_bhatt@intuit.com"
LABEL org.opencontainers.image.source="https://github.com/intuit/go-loadgen"
COPY --from=build /loadgen /loadgen
ENTRYPOINT ["/loadgen"]
