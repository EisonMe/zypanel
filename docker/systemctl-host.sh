#!/bin/sh
# Run systemctl against the host's systemd.
#
# The container is started with `pid: host` and `privileged: true`, so PID 1
# inside the container is the host's systemd. Entering its mount/pid/etc
# namespaces gives us the host binaries, the host D-Bus and /run/systemd/private,
# which is exactly what systemctl needs.
set -eu

if [ ! -e /run/systemd/private ]; then
  echo "systemctl-host: /run/systemd/private not found; mount /run and use pid: host" >&2
  exit 1
fi

exec nsenter -t 1 -m -u -i -n -p -- systemctl "$@"
