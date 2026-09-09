FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/geocoder ./cmd/geocoder

FROM debian:bookworm-slim
COPY --from=build /out/geocoder /usr/local/bin/geocoder
EXPOSE 8080
ENTRYPOINT ["geocoder"]
