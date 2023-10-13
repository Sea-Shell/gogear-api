FROM registry.hub.docker.com/library/golang:1.21-alpine as go-upx
RUN ["sh", "-exo", "pipefail", "-c", "apk add git upx; rm -vf /var/cache/apk/*"]
ENV CGO_ENABLED 0

##########################
FROM go-upx as builder

# Install git + SSL ca certificates.
# Git is required for fetching the dependencies.
# Ca-certificates is required to call HTTPS endpoints.
RUN apk update && apk add --no-cache ca-certificates tzdata && update-ca-certificates
RUN cp /usr/share/zoneinfo/Europe/Oslo /etc/localtime
RUN echo "Europe/Oslo" > /etc/timezone

# Create appuser
ENV USER=appuser
ENV UID=10001
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

COPY *.go      /app/
COPY go.mod    /app/
COPY go.sum    /app/
COPY pkg       /app/pkg


RUN go mod download
RUN go mod verify
RUN go mod tidy
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    ["go", "build", "-ldflags", "-s -w", "."]

RUN ["upx", "gogear-api"]

##########################
FROM scratch

WORKDIR /app

COPY --from=builder /etc/passwd                                   /etc/passwd
COPY --from=builder /etc/group                                    /etc/group
COPY --from=builder /etc/ssl/certs/ca-certificates.crt            /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo                           /usr/share/zoneinfo
COPY --from=builder /app/gogear-api            /app/gogear-api

USER appuser:appuser

EXPOSE 8080

ENTRYPOINT [ "/app/gogear-api" ]
