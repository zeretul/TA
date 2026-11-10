#!/bin/bash
set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${ROOT_DIR}"

OUTPUT_DIR="output"
VERSION="${TA_VERSION:-${DENOVA_VERSION:-${NOVA_VERSION:-$(node -p "require('./web/package.json').version" 2>/dev/null || echo dev)}}}"

echo "==> 清理 output 目录"
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

EMBED_TAG=""
echo "==> 构建前端"
if [ -d "web" ]; then
    cd web
    if [ ! -d "node_modules" ]; then
        echo "  安装依赖..."
        pnpm install
    fi
    pnpm build
    cd ..
    echo "  复制前端产物到 ${OUTPUT_DIR}/web/"
    cp -r web/dist "${OUTPUT_DIR}/web"
    echo "  准备内嵌前端资源（go:embed，构建标签 embedweb）"
    rm -rf internal/webfs/dist
    cp -r web/dist internal/webfs/dist
    EMBED_TAG="-tags embedweb"
else
    echo "警告: web/ 目录不存在，跳过前端构建（TA 将不含内嵌前端）"
fi

echo "==> 编译 TA"
go build ${EMBED_TAG} -ldflags "-X ta/internal/buildinfo.Version=${VERSION}" -o "${OUTPUT_DIR}/TA" ./cmd/ta/

echo "==> 编译 TA-updater"
go build -ldflags "-X ta/internal/buildinfo.Version=${VERSION}" -o "${OUTPUT_DIR}/TA-updater" ./cmd/ta-updater/

echo "==> 打包内置 ripgrep"
RIPGREP_GOOS="$(go env GOOS)"
RIPGREP_GOARCH="$(go env GOARCH)"
RIPGREP_HOST_GOOS="$(go env GOHOSTOS)"
RIPGREP_HOST_GOARCH="$(go env GOHOSTARCH)"
case "${RIPGREP_GOOS}-${RIPGREP_GOARCH}" in
    darwin-arm64) RIPGREP_TARGET="darwin-arm64" ;;
    darwin-amd64) RIPGREP_TARGET="darwin-x64" ;;
    linux-arm64) RIPGREP_TARGET="linux-arm64" ;;
    linux-amd64) RIPGREP_TARGET="linux-x64" ;;
    windows-amd64) RIPGREP_TARGET="windows-x64" ;;
    *)
        echo "错误: 当前平台没有 TA 内置 ripgrep 产物: ${RIPGREP_GOOS}-${RIPGREP_GOARCH}" >&2
        echo "Error: no bundled TA ripgrep asset is available for ${RIPGREP_GOOS}-${RIPGREP_GOARCH}" >&2
        exit 1
        ;;
esac
GOOS="${RIPGREP_HOST_GOOS}" GOARCH="${RIPGREP_HOST_GOARCH}" go run ./scripts/ripgrep-assets \
    -target "${RIPGREP_TARGET}" \
    -destination "${OUTPUT_DIR}"

echo "==> 复制 skills 目录"
cp -r skills "${OUTPUT_DIR}/skills"

# The working config.toml is private and must never enter a distributable build.
echo "==> Copying the unconfigured startup template"
cp config.example.toml "${OUTPUT_DIR}/config.toml"

echo "==> 复制 CHANGELOG.md"
if [ -f CHANGELOG.md ]; then
    cp CHANGELOG.md "${OUTPUT_DIR}/CHANGELOG.md"
else
    echo "警告: CHANGELOG.md 不存在，跳过复制"
fi

echo "==> 构建完成"
echo "  输出目录: ${OUTPUT_DIR}/"
ls -la "${OUTPUT_DIR}/"
echo ""
echo "使用方式:"
echo "  cd ${OUTPUT_DIR} && ./TA --workspace /path/to/my-novel"
