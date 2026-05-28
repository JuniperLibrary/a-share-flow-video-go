FROM golang:1.25-alpine AS builder

RUN apk --no-cache add git ca-certificates

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 修改绑定地址为 0.0.0.0（容器内需要监听所有网卡）
RUN sed -i 's/127.0.0.1/0.0.0.0/g' cmd/web/main.go

RUN CGO_ENABLED=0 go build -o web-server ./cmd/web/

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata wget

ENV TZ=Asia/Shanghai

WORKDIR /app

COPY --from=builder /app/web-server .
COPY --from=builder /app/.env.example .env 2>/dev/null || true

RUN mkdir -p /app/data /app/output

EXPOSE 8084

HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=10s \
  CMD wget -qO- http://127.0.0.1:8084/api/dates || exit 1

ENTRYPOINT ["./web-server"]
