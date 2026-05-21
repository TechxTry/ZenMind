#!/usr/bin/env bash
# ZenMind 统一版本号：Harbor / Docker Hub / GitHub Release / 前端展示 共用 VERSION 文件
#
# 格式：v + 多段数字（至少 2 段，常见 vMAJOR.MINOR.PATCH，也可 v1.0.44.1）
#
# 用法：
#   ./scripts/version.sh current          # 输出当前版本（无换行装饰）
#   ./scripts/version.sh set v1.0.45    # 写入 VERSION 并同步 .harbor-versions
#   ./scripts/version.sh bump patch     # 末段 +1（默认发布递增方式）
#   ./scripts/version.sh bump minor     # 次版本 +1，其后段归零
#   ./scripts/version.sh bump major     # 主版本 +1，其余段归零
#   ./scripts/version.sh sync           # 仅根据 VERSION 重写 .harbor-versions
#   ./scripts/version.sh tag-git        # 以当前 VERSION 创建并推送 git 标签（需已 commit）

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_FILE="${ROOT}/VERSION"
HARBOR_VERSIONS_FILE="${ROOT}/.harbor-versions"

VERSION_RE='^v[0-9]+(\.[0-9]+)+$'

die() { echo "version.sh: $*" >&2; exit 1; }

read_version() {
  if [[ ! -f "$VERSION_FILE" ]]; then
    die "缺少 ${VERSION_FILE}，请先创建，例如：echo v1.0.0 > VERSION"
  fi
  local v
  v="$(tr -d '[:space:]' < "$VERSION_FILE")"
  [[ "$v" =~ $VERSION_RE ]] || die "无效版本格式: ${v}（期望如 v1.0.44 或 v1.0.44.1）"
  echo "$v"
}

write_version() {
  local v="$1"
  [[ "$v" =~ $VERSION_RE ]] || die "无效版本: ${v}"
  printf '%s\n' "$v" > "$VERSION_FILE"
  sync_harbor_versions "$v"
  echo "$v"
}

sync_harbor_versions() {
  local v="$1"
  cat > "$HARBOR_VERSIONS_FILE" <<EOF
# 由 scripts/version.sh 根据 VERSION 自动生成，与 Harbor/Docker/Git 标签保持一致
backend=${v}
frontend=${v}
EOF
}

# 将 v1.2.3.4 转为数组并 bump
bump_version() {
  local v="$1"
  local level="${2:-patch}"
  v="${v#v}"
  local -a parts
  IFS='.' read -r -a parts <<< "$v"
  local n="${#parts[@]}"
  (( n >= 2 )) || die "版本至少需要两段数字"

  case "$level" in
    patch|build)
      parts[$((n - 1))]=$((parts[$((n - 1))] + 1))
      ;;
    minor)
      if (( n < 2 )); then die "无法 bump minor"; fi
      local mi=$((n - 2))
      parts[$mi]=$((parts[$mi] + 1))
      for ((i = mi + 1; i < n; i++)); do parts[i]=0; done
      ;;
    major)
      parts[0]=$((parts[0] + 1))
      for ((i = 1; i < n; i++)); do parts[i]=0; done
      ;;
    *)
      die "未知 bump 级别: ${level}（可选 patch|minor|major）"
      ;;
  esac

  local joined=""
  local i
  for ((i = 0; i < n; i++)); do
    [[ -n "$joined" ]] && joined+="."
    joined+="${parts[$i]}"
  done
  echo "v${joined}"
}

cmd="${1:-current}"
arg="${2:-}"

case "$cmd" in
  current)
    read_version
    ;;
  set)
    [[ -n "$arg" ]] || die "用法: version.sh set v1.0.45"
    write_version "$arg"
    ;;
  bump)
    cur="$(read_version)"
    next="$(bump_version "$cur" "${arg:-patch}")"
    write_version "$next"
    ;;
  sync)
    v="$(read_version)"
    sync_harbor_versions "$v"
    echo "$v"
    ;;
  tag-git)
    v="$(read_version)"
    if git -C "$ROOT" rev-parse "$v" >/dev/null 2>&1; then
      die "标签 ${v} 已存在"
    fi
    git -C "$ROOT" tag -a "$v" -m "ZenMind ${v}"
    echo "已创建标签 ${v}，推送请执行: git push origin ${v}"
    ;;
  *)
    die "未知命令: ${cmd}（current|set|bump|sync|tag-git）"
    ;;
esac
