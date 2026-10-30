#!/bin/sh
set -eu

if [ "${RENEWED_LINEAGE:-}" = "/etc/letsencrypt/live/wb.sshzyu.com" ]; then
    /usr/sbin/nginx -t
    /usr/sbin/nginx -s reload
fi
