FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/geocoder ./cmd/geocoder
RUN CGO_ENABLED=0 go build -o /out/importer ./cmd/importer

FROM debian:bookworm-slim
COPY --from=build /out/geocoder /usr/local/bin/geocoder
COPY --from=build /out/importer /usr/local/bin/importer
