#!/usr/bin/env bash
# Void Architecture™ Kernel Confinement Perimeter
set -euo pipefail

JAIL_DIR="${JAIL_DIR:-/run/void-jail}"
APP_BIN="${APP_BIN:-./bin/app}"
HOST_DATA_DIR="${HOST_DATA_DIR:-$(pwd)/data}"
TMPFS_SIZE="${TMPFS_SIZE:-64M}"

if [[ $EUID -ne 0 ]]; then
    echo "[-] Void containment requires root privileges." >&2
    exit 1
fi

mkdir -p "${JAIL_DIR}"
mount -t tmpfs -o "size=${TMPFS_SIZE},nodev,nosuid" tmpfs "${JAIL_DIR}"

mkdir -p "${JAIL_DIR}"/{bin,data,lib,lib64,proc,dev,etc,tmp}
cp "${APP_BIN}" "${JAIL_DIR}/bin/app"
chmod 755 "${JAIL_DIR}/bin/app"

mknod -m 666 "${JAIL_DIR}/dev/null" c 1 3 2>/dev/null || true
mknod -m 666 "${JAIL_DIR}/dev/zero" c 1 5 2>/dev/null || true
mknod -m 666 "${JAIL_DIR}/dev/urandom" c 1 9 2>/dev/null || true

[[ -f /etc/resolv.conf ]] && cp -a /etc/resolv.conf "${JAIL_DIR}/etc/" 2>/dev/null || true

mkdir -p "${HOST_DATA_DIR}"
mount --bind "${HOST_DATA_DIR}" "${JAIL_DIR}/data"

cleanup() {
    umount -l "${JAIL_DIR}/data" 2>/dev/null || true
    umount -l "${JAIL_DIR}" 2>/dev/null || true
    rm -rf "${JAIL_DIR}"
}
trap cleanup EXIT INT TERM

exec unshare --mount --pid --ipc --uts --fork chroot "${JAIL_DIR}" /bin/app
