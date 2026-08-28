#!/bin/bash

set -euo pipefail

# script to create debbuild.tar.gz and upload to pigsty.cc / pigsty.io

PROG_NAME="$(basename "$0")"
BIN_DIR="$(cd "$(dirname "$0")" && pwd)"
HOME_DIR="$(cd "${BIN_DIR}/.." && pwd)"

if [[ $# -ne 1 ]]; then
  echo "usage: ${PROG_NAME} vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

PIG_VERSION="$1"
ARTIFACT_VERSION="${PIG_VERSION#v}"
SOURCE_DIR="${HOME_DIR}/dist"
TARGET_DIR=~/pgsty/repo/pkg/pig/${PIG_VERSION}

if [[ ! "${PIG_VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: ${PROG_NAME} vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

ARTIFACTS=(
  "pig-v${ARTIFACT_VERSION}.darwin-amd64.tar.gz"
  "pig-v${ARTIFACT_VERSION}.darwin-arm64.tar.gz"
  "pig-v${ARTIFACT_VERSION}.linux-amd64.tar.gz"
  "pig-v${ARTIFACT_VERSION}.linux-arm64.tar.gz"
  "pig-${ARTIFACT_VERSION}-1.x86_64.rpm"
  "pig-${ARTIFACT_VERSION}-1.aarch64.rpm"
  "pig_${ARTIFACT_VERSION}-1_amd64.deb"
  "pig_${ARTIFACT_VERSION}-1_arm64.deb"
  "checksums.txt"
)

for artifact in "${ARTIFACTS[@]}"; do
  if [[ ! -f "${SOURCE_DIR}/${artifact}" ]]; then
    echo "missing release artifact: ${SOURCE_DIR}/${artifact}" >&2
    exit 1
  fi
done

echo "SRC: ${SOURCE_DIR}"
ls -alh "${SOURCE_DIR}"

echo "DST: ${TARGET_DIR}"
if [[ -e "${TARGET_DIR}" ]]; then
  echo "refusing to overwrite existing release directory: ${TARGET_DIR}" >&2
  exit 1
fi
mkdir -p "${TARGET_DIR}"
ls -alh "${TARGET_DIR}"

echo "copy tgz, rpm, deb, and checksums to target dir"
for artifact in "${ARTIFACTS[@]}"; do
  cp -n "${SOURCE_DIR}/${artifact}" "${TARGET_DIR}/"
done

(cd "${TARGET_DIR}" && shasum -a 256 -c checksums.txt)


echo "upload to pigsty.cc"
echo rclone copy "${TARGET_DIR}/" "cos:/repo-1304744452/pkg/pig/${PIG_VERSION}/" --immutable --progress
rclone copy "${TARGET_DIR}/" "cos:/repo-1304744452/pkg/pig/${PIG_VERSION}/" --immutable --progress

echo "upload to pigsty.io"
echo rclone copy "${TARGET_DIR}/" "cf:/repo/pkg/pig/${PIG_VERSION}/" --immutable --progress
rclone copy "${TARGET_DIR}/" "cf:/repo/pkg/pig/${PIG_VERSION}/" --immutable --progress
