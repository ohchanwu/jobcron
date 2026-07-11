# Demo/development image used by deploy/demo.
# Production may reuse the binary build idea, but this Dockerfile is currently
# documented and wired for the read-only demo deployment, not the private
# jobcron.app production stack.
FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/jobcron ./cmd/jobcron

FROM alpine:3.22

RUN apk add --no-cache ca-certificates
COPY --from=build /out/jobcron /usr/local/bin/jobcron

EXPOSE 7777
ENTRYPOINT ["jobcron"]
