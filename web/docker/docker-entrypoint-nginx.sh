#!/bin/sh
# [Ref: 04_Phase4/01_成本透视真实数据] 启动时注入 resolver，使 backend 在请求时解析，避免 compose 启动顺序导致 nginx [emerg] host not found
# 从 /etc/resolv.conf 取第一个 nameserver（Podman 等）；Docker 下可为 127.0.0.11；可通过 env NGINX_RESOLVER 覆盖
set -e
if [ -z "$NGINX_RESOLVER" ]; then
  NGINX_RESOLVER=$(grep -m1 '^nameserver ' /etc/resolv.conf 2>/dev/null | awk '{print $2}' || true)
fi
NGINX_RESOLVER="${NGINX_RESOLVER:-127.0.0.11}"
sed -i "s/__NGINX_RESOLVER__/$NGINX_RESOLVER/g" /etc/nginx/conf.d/default.conf
exec nginx -g 'daemon off;'
