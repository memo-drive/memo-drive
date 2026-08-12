FROM golang:1.26-alpine AS build

WORKDIR /src
RUN apk add --no-cache build-base
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /out/memodrive ./cmd/server
RUN CGO_ENABLED=1 GOOS=linux go build -o /out/memodrive-admin ./cmd/memodrive-admin

FROM alpine:3.21

WORKDIR /app
RUN apk add --no-cache ca-certificates ffmpeg wget \
    tesseract-ocr tesseract-ocr-data-eng tesseract-ocr-data-chi_sim
COPY --from=build /out/memodrive /app/memodrive
COPY --from=build /out/memodrive-admin /app/memodrive-admin
EXPOSE 8080
CMD ["/app/memodrive"]
