#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist/github-release"
BUILD_DIR="${DIST_DIR}/build"
VERSION="${1:-${GITHUB_REF_NAME:-}}"
TARGET="${2:-all}"
# CI owns verification. Packaging consumes its exact-revision frontend artifact;
# standalone builds prepare the frontend once but do not rerun the test suites.
FRONTEND_DIR="${TA_RELEASE_FRONTEND_DIR:-${ROOT_DIR}/web/dist}"
# Set only in the trusted release build; players cannot change this channel.
UPDATE_REPOSITORY="${TA_UPDATE_REPOSITORY:-}"
if [[ -n "${UPDATE_REPOSITORY}" ]] && { [[ ! "${UPDATE_REPOSITORY}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || [[ "${UPDATE_REPOSITORY,,}" == "alfredxw/denova" ]] || [[ "${UPDATE_REPOSITORY%%/*}" == . || "${UPDATE_REPOSITORY%%/*}" == .. || "${UPDATE_REPOSITORY#*/}" == . || "${UPDATE_REPOSITORY#*/}" == .. ]]; }; then
  echo "Error: invalid TA update repository" >&2
  exit 1
fi

if [[ -z "${VERSION}" ]]; then
  if git -C "${ROOT_DIR}" describe --tags --exact-match >/dev/null 2>&1; then
    VERSION="$(git -C "${ROOT_DIR}" describe --tags --exact-match)"
  else
    VERSION="dev"
  fi
fi

TARGETS=(
  "darwin-arm64:darwin:arm64:TA:TA-updater:tar.gz"
  "darwin-x64:darwin:amd64:TA:TA-updater:tar.gz"
  "linux-arm64:linux:arm64:TA:TA-updater:tar.gz"
  "linux-x64:linux:amd64:TA:TA-updater:tar.gz"
  "windows-x64:windows:amd64:TA.exe:TA-updater.exe:zip"
)

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Error: command not found: $1" >&2
    exit 1
  fi
}

run_pnpm() {
  if command -v pnpm >/dev/null 2>&1; then
    pnpm "$@"
    return
  fi
  npx pnpm "$@"
}

copy_if_exists() {
  local from="$1"
  local to="$2"
  if [[ -e "${from}" ]]; then
    cp -R "${from}" "${to}"
  fi
}

checksum_file() {
  local file="$1"
  local name
  name="$(basename "${file}")"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${file}" | awk -v name="${name}" '{print $1 "  " name}'
    return
  fi
  shasum -a 256 "${file}" | awk -v name="${name}" '{print $1 "  " name}'
}

release_version_without_prefix() {
  printf '%s' "${VERSION#v}"
}

extract_release_brief() {
  local release_tag="$1"
  awk -v release_heading="## [${release_tag}]" '
    index($0, release_heading) == 1 { in_release = 1; next }
    in_release && /^## \[/ { exit }
    in_release && $0 == "### Brief / 简要说明" { in_brief = 1; found = 1; next }
    in_brief && /^### / { exit }
    in_brief { print }
    END {
      if (!found) {
        print "CHANGELOG.md is missing a Brief section for " release_heading > "/dev/stderr"
        exit 1
      }
    }
  '
}

validate_release_metadata() {
  if [[ "${VERSION}" == "dev" ]]; then
    return
  fi
  local expected web_version release_tag
  expected="$(release_version_without_prefix)"
  release_tag="v${expected}"
  web_version="$(node -p "require('./web/package.json').version")"
  if [[ "${web_version}" != "${expected}" ]]; then
    echo "Error: release ${release_tag} does not match web version ${web_version}" >&2
    exit 1
  fi
  if ! grep -Fq "## [${release_tag}]" CHANGELOG.md; then
    echo "Error: CHANGELOG.md is missing ${release_tag}" >&2
    exit 1
  fi
  extract_release_brief "${release_tag}" < CHANGELOG.md >/dev/null
  if ! grep -Fq "<strong>${release_tag}</strong>" README.md || ! grep -Fq "<strong>${release_tag}</strong>" README.en.md; then
    echo "Error: both README files must identify ${release_tag} as the current version" >&2
    exit 1
  fi
}

write_release_notes() {
  local release_tag
  release_tag="v$(release_version_without_prefix)"
  {
    echo "# Text Adventure ${release_tag}"
    echo
    echo "## Release highlights / 发布内容"
    echo
    extract_release_brief "${release_tag}" < CHANGELOG.md
    cat <<'EOF'

## Verification / 验证

- The release workflow requires successful CI for the exact tagged commit, including both Go modules, frontend unit tests, translations, and browser journeys against the built distribution.
- Packaging reuses the frontend from that CI run; platform archives are compiled in parallel without repeating the test suites.
- Packaging: five platform archives are generated from the same source revision and listed in `checksums.txt`.

发布流程要求标签对应提交的 CI 成功，覆盖两个 Go module、前端单测、双语键检查、生产构建及构建产物上的浏览器流程；复用该次 CI 的前端产物，并行生成五个平台压缩包及 `checksums.txt`。

## Install / 安装

Download the archive for your platform from this release, verify it against `checksums.txt`, extract it, and run Text Adventure from the extracted `TA` directory.

请从本次发布下载对应平台压缩包，使用 `checksums.txt` 校验后解压，并在解压后的 `TA` 目录运行：

```bash
./TA
```

Windows:

```powershell
TA.exe
```

Checksum example / 校验示例：

```bash
shasum -a 256 -c checksums.txt
```
EOF
  } > "${DIST_DIR}/RELEASE_NOTES.md"
}

require_command go
require_command node
require_command tar

if [[ ! "${VERSION}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
  echo "Error: invalid release version: ${VERSION}" >&2
  exit 1
fi
if [[ "${TARGET}" != all ]]; then
  selected=()
  for target in "${TARGETS[@]}"; do
    if [[ "${target%%:*}" == "${TARGET}" ]]; then selected+=("${target}"); fi
  done
  if [[ ${#selected[@]} -ne 1 ]]; then
    echo "Error: unsupported release target: ${TARGET}" >&2
    exit 1
  fi
  TARGETS=("${selected[@]}")
fi

echo "==> Building GitHub Release assets version=${VERSION} target=${TARGET}"
cd "${ROOT_DIR}"
validate_release_metadata
if [[ -z "${TA_RELEASE_FRONTEND_DIR:-}" ]]; then
  echo "==> Preparing frontend (run CI separately before publishing)"
  run_pnpm -C "${ROOT_DIR}/web" install --frozen-lockfile
  run_pnpm -C "${ROOT_DIR}/web" check:i18n
  run_pnpm -C "${ROOT_DIR}/web" build
fi
if [[ ! -s "${FRONTEND_DIR}/index.html" ]]; then
  echo "Error: missing built frontend: ${FRONTEND_DIR}/index.html" >&2
  exit 1
fi
mkdir -p "${DIST_DIR}" "${BUILD_DIR}"
# The upstream installer points at a different product. Do not publish a stale copy.
rm -f "${DIST_DIR}/install.sh"
# Match the normal distribution: serve external assets and retain the embedded
# fallback. Only generated directories below this checkout are replaced.
rm -rf "${ROOT_DIR}/internal/webfs/dist"
cp -R "${FRONTEND_DIR}" "${ROOT_DIR}/internal/webfs/dist"

echo "==> Compiling and packaging"
for target in "${TARGETS[@]}"; do
  IFS=":" read -r key goos goarch exe updater_exe archive_type <<<"${target}"
  package_name="TA-${VERSION}-${key}"
  package_dir="${BUILD_DIR}/${package_name}/TA"
  rm -rf "${BUILD_DIR}/${package_name}"
  mkdir -p "${package_dir}"

  echo "  -> ${key}"
  binary_version="${VERSION#v}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -tags embedweb -trimpath -ldflags "-s -w -X ta/internal/buildinfo.Version=${binary_version} -X ta/internal/buildinfo.UpdateRepository=${UPDATE_REPOSITORY}" -o "${package_dir}/${exe}" ./cmd/ta
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -trimpath -ldflags "-s -w -X ta/internal/buildinfo.Version=${binary_version}" -o "${package_dir}/${updater_exe}" ./cmd/ta-updater

  ripgrep_args=(-target "${key}" -destination "${package_dir}")
  if [[ -n "${TA_RIPGREP_BASE_URL:-}" ]]; then
    ripgrep_args+=(-base-url "${TA_RIPGREP_BASE_URL}")
  fi
  go run ./scripts/ripgrep-assets "${ripgrep_args[@]}"

  if [[ "${goos}" != "windows" ]]; then
    chmod 0755 "${package_dir}/${exe}"
    chmod 0755 "${package_dir}/${updater_exe}"
  fi

  cp -R "${FRONTEND_DIR}" "${package_dir}/web"
  cp -R "${ROOT_DIR}/skills" "${package_dir}/skills"
  # Always package the credential-free template, never the developer config.
  cp "${ROOT_DIR}/config.example.toml" "${package_dir}/config.toml"
  copy_if_exists "${ROOT_DIR}/README.md" "${package_dir}/"
  copy_if_exists "${ROOT_DIR}/README.en.md" "${package_dir}/"
  copy_if_exists "${ROOT_DIR}/CHANGELOG.md" "${package_dir}/"
  copy_if_exists "${ROOT_DIR}/LICENSE" "${package_dir}/"
  # Apache-2.0 section 4(b) requires a modified distribution to carry the
  # upstream notices and a statement of the changes.
  copy_if_exists "${ROOT_DIR}/NOTICE" "${package_dir}/"

  # The archive holds a compiled binary and a built frontend, so the dependency
  # license files that sit next to the sources never reach the user. Collect
  # them per target: the dependency set is GOOS-specific.
  module_list="${BUILD_DIR}/${package_name}/go-modules.txt"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}{{end}}' \
    ./cmd/ta ./cmd/ta-updater | sed '/^$/d' | sort -u > "${module_list}"
  licenses_args=(-modules "${module_list}" -repo "${ROOT_DIR}" -destination "${package_dir}" -version "${binary_version}")
  if [[ -d "${ROOT_DIR}/web/node_modules" ]]; then
    licenses_args+=(-web "${ROOT_DIR}/web")
  else
    echo "  Warning: web/node_modules is absent, so the frontend notices are omitted from ${key}" >&2
  fi
  go run ./scripts/release-licenses "${licenses_args[@]}"

  if [[ "${archive_type}" == "zip" ]]; then
    rm -f "${DIST_DIR}/${package_name}.zip"
    (
      cd "${BUILD_DIR}/${package_name}"
      if command -v zip >/dev/null 2>&1; then
        zip -qr "${DIST_DIR}/${package_name}.zip" TA
      elif command -v python3 >/dev/null 2>&1; then
        python3 -m zipfile -c "${DIST_DIR}/${package_name}.zip" TA
      elif command -v python >/dev/null 2>&1; then
        python -m zipfile -c "${DIST_DIR}/${package_name}.zip" TA
      else
        echo "Error: zip or Python is required to create the Windows archive" >&2
        exit 1
      fi
    )
  else
    (
      cd "${BUILD_DIR}/${package_name}"
      tar -czf "${DIST_DIR}/${package_name}.tar.gz" TA
    )
  fi
done

echo "==> Writing checksums.txt"
: > "${DIST_DIR}/checksums.txt"
for file in "${DIST_DIR}"/TA-"${VERSION}"-*; do
  checksum_file "${file}" >> "${DIST_DIR}/checksums.txt"
done

write_release_notes

echo "==> GitHub Release assets ready: ${DIST_DIR}"
find "${DIST_DIR}" -maxdepth 1 -type f -print | sort
