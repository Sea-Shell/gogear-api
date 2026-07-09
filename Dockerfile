FROM docker.io/library/golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 as go-upx
RUN ["sh", "-exo", "pipefail", "-c", "apk add git upx; rm -vf /var/cache/apk/*"]
ENV CGO_ENABLED 1

##########################
FROM go-upx as builder

# Install git + SSL ca certificates.
# Git is required for fetching the dependencies.
# Ca-certificates is required to call HTTPS endpoints.
RUN apk update && apk add --no-cache gcc libc-dev ca-certificates tzdata && update-ca-certificates
RUN cp /usr/share/zoneinfo/Europe/Oslo /etc/localtime
RUN echo "Europe/Oslo" > /etc/timezone

# Create appuser
ENV USER=abc
ENV UID=1001
# See https://stackoverflow.com/a/55757473/12429735RUN 
RUN adduser \    
    --disabled-password \    
    --gecos "" \    
    --home "/nonexistent" \    
    --shell "/sbin/nologin" \    
    --no-create-home \    
    --uid "${UID}" \    
    "${USER}"

WORKDIR /app

COPY main.go   /app/
COPY go.mod    /app/
COPY go.sum    /app/
COPY pkg       /app/pkg
COPY docs      /app/docs

ENV GOPRIVATE=github.com/Sea-Shell/gogear-api

RUN go mod download
RUN go mod verify
RUN go mod tidy -e
RUN go build -o gogear-api -ldflags="-s -w"
RUN chmod +x /app/gogear-api

RUN ["upx", "-q", "gogear-api"]

##########################
FROM docker.io/library/alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

WORKDIR /app

COPY --from=builder /etc/passwd                        /etc/passwd
COPY --from=builder /etc/group                         /etc/group
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo                /usr/share/zoneinfo
COPY --from=builder /app/gogear-api                    /app/gogear-api
COPY --from=builder /bin/sh                            /bin/sh
COPY migrations                                         /app/migrations

USER abc:abc

EXPOSE 8080

ENTRYPOINT [ "/app/gogear-api"]
