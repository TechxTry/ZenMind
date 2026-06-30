#!/usr/bin/env bash
# Build and publish ZenMind images to Docker Hub.
#
# Defaults:
#   namespace: techxtry
#   services:  backend frontend
#   tag:       VERSION patch +1 (when omitted)
#
# Examples:
#   ./scripts/dockerhub-publish.sh
#   ./scripts/dockerhub-publish.sh backend
#   ./scripts/dockerhub-publish.sh all v1.0.58
#   DOCKERHUB_USERNAME=techxtry DOCKERHUB_PASSWORD=*** ./scripts/dockerhub-publish.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_SCRIPT="${ROOT_DIR}/scripts/version.sh"

DOCKERHUB_NAMESPACE="${DOCKERHUB_NAMESPACE:-techxtry}"
BUILD_PLATFORM="${BUILD_PLATFORM:-linux/amd64}"
DOCKERHUB_AUTH_IPV4="${DOCKERHUB_AUTH_IPV4:-31.13.95.48}"

SERVICES=("backend" "frontend")

log() {
  printf '[dockerhub] %s\n' "$*"
}

die() {
  printf '[dockerhub] ERROR: %s\n' "$*" >&2
  exit 1
}

bump_patch_version() {
  if [[ -x "$VERSION_SCRIPT" ]]; then
    "$VERSION_SCRIPT" bump patch
    return
  fi
  die "missing executable ${VERSION_SCRIPT}; pass an explicit tag"
}

image_name_for() {
  local service="$1"
  local tag="$2"
  printf '%s/zenmind-%s:%s' "$DOCKERHUB_NAMESPACE" "$service" "$tag"
}

check_service() {
  local service="$1"
  [[ "$service" == "all" || "$service" == "backend" || "$service" == "frontend" ]] || \
    die "unknown service: ${service} (expected backend, frontend, or all)"
}

login_if_configured() {
  if [[ -n "${DOCKERHUB_PASSWORD:-}" ]]; then
    local user="${DOCKERHUB_USERNAME:-$DOCKERHUB_NAMESPACE}"
    log "logging in to Docker Hub as ${user}"
    printf '%s' "$DOCKERHUB_PASSWORD" | docker login -u "$user" --password-stdin
    return
  fi
  log "using existing Docker Hub login (set DOCKERHUB_PASSWORD to login non-interactively)"
}

build_service() {
  local service="$1"
  local tag="$2"
  local image
  image="$(image_name_for "$service" "$tag")"

  log "building ${image} (platform=${BUILD_PLATFORM})"
  case "$service" in
    backend)
      docker buildx build \
        --network host \
        --add-host "auth.docker.io:${DOCKERHUB_AUTH_IPV4}" \
        --platform "$BUILD_PLATFORM" \
        -t "$image" \
        --load \
        -f "${ROOT_DIR}/backend/Dockerfile" \
        "${ROOT_DIR}/backend"
      ;;
    frontend)
      docker buildx build \
        --network host \
        --add-host "auth.docker.io:${DOCKERHUB_AUTH_IPV4}" \
        --platform "$BUILD_PLATFORM" \
        --build-arg "APP_VERSION=${tag}" \
        -t "$image" \
        --load \
        -f "${ROOT_DIR}/frontend/Dockerfile" \
        "${ROOT_DIR}/frontend"
      ;;
  esac

  docker tag "$image" "$(image_name_for "$service" latest)"
}

push_service() {
  local service="$1"
  local tag="$2"

  docker push "$(image_name_for "$service" "$tag")"
  docker push "$(image_name_for "$service" latest)"
}

main() {
  local service="${1:-all}"
  local tag="${2:-}"
  check_service "$service"

  if [[ -z "$tag" ]]; then
    tag="$(bump_patch_version)"
    log "auto bumped VERSION to ${tag}"
  else
    log "using explicit tag ${tag}"
  fi

  docker info >/dev/null
  login_if_configured

  log "publishing service=${service} tag=${tag} namespace=${DOCKERHUB_NAMESPACE}"
  if [[ "$service" == "all" ]]; then
    for svc in "${SERVICES[@]}"; do
      build_service "$svc" "$tag"
      push_service "$svc" "$tag"
    done
  else
    build_service "$service" "$tag"
    push_service "$service" "$tag"
  fi
  log "published ${tag}"
}

main "$@"
