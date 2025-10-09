# Build stage
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/qazna-api ./cmd/api

# Run stage
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/qazna-api /qazna-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/qazna-api"]
