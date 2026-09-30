FROM golang:1.27.0-alpine

WORKDIR /app

# 安装必要的工具
RUN apk add --no-cache git

# 复制go.mod和go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用：正式发布需显式传入与 cmd/server/VERSION 一致的完整本地版本及 Git SHA。
ARG VERSION=
ARG COMMIT=unknown
ARG DATE=unknown
RUN VERSION_VALUE="${VERSION:-$(./scripts/resolve-version.sh)}" && \
    test "${VERSION_VALUE}" = "$(sh ./scripts/resolve-version.sh)" && \
    printf '%s' "${VERSION_VALUE}" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+-r[1-9][0-9]*$' && \
    go build -ldflags="-s -w -X main.Version=${VERSION_VALUE} -X main.Commit=${COMMIT} -X main.Date=${DATE}" -o main ./cmd/server/

# 暴露端口
EXPOSE 8080

# 运行应用
CMD ["./main"]
