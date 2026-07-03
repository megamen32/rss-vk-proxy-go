# rss-vk-proxy-go

Direct Telegram public-channel RSS proxy with VK-friendly plain-text output.

It replaces the earlier RSSHub-based Python normalizer by fetching `https://t.me/s/<channel>` directly, parsing Telegram public HTML, and exposing RSS at:

```text
/rssvk/telegram/channel/<channel>?mode=vk
```

## Features

- No RSSHub dependency.
- Plain-text RSS descriptions suitable for VK import.
- Preserves Telegram paragraph breaks and blockquote text as normal readable paragraphs.
- Expands visible links as `text (url)` by default; use `links=0` to suppress URL expansion.
- Supports `limit`, `source=1`, and `mode=vk` query parameters.
- Caches Telegram HTML locally and can serve stale cache if Telegram fetch fails.
- Uses standard `HTTP_PROXY` / `HTTPS_PROXY` environment variables.

## Run

```bash
go build -o rss-vk-proxy-go ./main.go
RSSVK_CACHE_DIR=./cache \
HTTPS_PROXY=http://192.168.2.1:3128 \
HTTP_PROXY=http://192.168.2.1:3128 \
./rss-vk-proxy-go 127.0.0.1:18766
```

Healthcheck:

```bash
curl http://127.0.0.1:18766/healthz
```

Example:

```bash
curl 'http://127.0.0.1:18766/rssvk/telegram/channel/bezrabotnyi?mode=vk&limit=3'
```

## Production notes

The current production deployment is usually proxied by nginx under `/rssvk/` and run by `rss-vk-proxy.service`.
Do not commit `cache/` or the compiled binary: Telegram cache files can contain temporary CDN tokens.
