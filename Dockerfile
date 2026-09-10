FROM oven/bun:1.3.14-alpine AS web
WORKDIR /app/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
COPY campus/locales /app/campus/locales
RUN bun run build

FROM golang:1.25.1-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY campus ./campus
COPY common ./common
COPY main.go .
ARG REVISION=development
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.revision=${REVISION}" -o /out/platform .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S -g 10001 platform && adduser -S -D -H -u 10001 -G platform platform
WORKDIR /app
COPY --from=build /out/platform /app/platform
COPY --from=web /app/web/dist /app/web/dist
COPY LICENSE /app/LICENSE
USER 10001:10001
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=60s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:3000/readyz || exit 1
ENTRYPOINT ["/app/platform"]
