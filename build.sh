#!/bin/sh
set -eu

VERSION=${VERSION:-dev}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags="-s -w \
    -X github.com/rfancn/zentao-mattermost/cmd.version=${VERSION} \
    -X github.com/rfancn/zentao-mattermost/cmd.commit=${COMMIT} \
    -X github.com/rfancn/zentao-mattermost/cmd.buildDate=${BUILD_DATE}" \
  -o zentao-mattermost .

echo "built ./zentao-mattermost"