FROM golang:1.26.5-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/semyonfox/seol/internal/app.version=${VERSION}" -o /seol ./cmd/seol

FROM alpine:3.22.3
RUN apk add --no-cache ca-certificates && addgroup -S seol && adduser -S -G seol seol
COPY --from=build /seol /usr/local/bin/seol
RUN mkdir /data && chown seol:seol /data
USER seol
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["seol"]
CMD ["serve"]
