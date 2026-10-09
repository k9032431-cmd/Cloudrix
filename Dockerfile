FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/k9032431-cmd/cloudrix/internal/api.Version=${VERSION}" -o /out/cloudrix ./cmd/cloudrix

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 cloudrix && mkdir -p /var/lib/cloudrix && chown cloudrix /var/lib/cloudrix
COPY --from=build /out/cloudrix /usr/local/bin/cloudrix
USER cloudrix
ENV CLOUDRIX_DB=/var/lib/cloudrix/cloudrix.db CLOUDRIX_LISTEN=:8000
VOLUME /var/lib/cloudrix
EXPOSE 8000
ENTRYPOINT ["cloudrix"]
