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

# 纯后端 Dockerfile 仅用于正式发布；缺失或不合法的构建身份直接拒绝。
ARG VERSION=
ARG COMMIT=
ARG DATE=
RUN test -n "${VERSION}" && \
    test "${VERSION}" = "$(sh ./scripts/resolve-version.sh)" && \
    printf '%s' "${VERSION}" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+-r[1-9][0-9]*$' && \
    printf '%s' "${COMMIT}" | grep -Eq '^[[:xdigit:]]{40}$' && \
    printf '%s' "${DATE}" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$' && \
    go build -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${DATE} -X main.BuildType=release" -o main ./cmd/server/

# 暴露端口
EXPOSE 8080

# 运行应用
CMD ["./main"]
