# Production image for the private jobcron.app deployment.
#
# Build this on the MacBook and push it to a registry. Do not build this image
# on the small EC2 production host.
FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/job-scraper ./cmd/job-scraper

FROM alpine:3.22

RUN apk add --no-cache ca-certificates
COPY --from=build /out/job-scraper /usr/local/bin/job-scraper

EXPOSE 7777
ENTRYPOINT ["job-scraper"]
