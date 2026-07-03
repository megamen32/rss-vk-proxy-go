# rss-vk-proxy-go

A small Go service that turns public Telegram channel pages into VK-friendly RSS feeds — without RSSHub.

It fetches Telegram's public web view (`https://t.me/s/<channel>`), extracts posts, converts Telegram HTML into readable plain text, and serves an RSS 2.0 feed that is easier for VK importers and other simple RSS consumers to digest.

## Why

RSSHub is great, but for this use case it was one more external dependency. This service is designed to be self-hosted, fast, boring, and predictable:

- direct Telegram public-page fetches;
- plain-text descriptions instead of heavy HTML;
- paragraph breaks preserved;
- blockquote text preserved as normal readable paragraphs;
- Telegram link previews dropped to avoid noisy duplicate text;
- optional stale-cache fallback when Telegram fetch fails;
- standard `HTTP_PROXY` / `HTTPS_PROXY` support.

## URL format

```text
/rssvk/telegram/channel/<channel>?mode=vk
```

Example:

```bash
curl 'http://127.0.0.1:18766/rssvk/telegram/channel/durov?mode=vk&limit=3'
```

## Query parameters

| Parameter | Default | Description |
| --- | --- | --- |
| `mode` | `vk` | Currently optimized for VK-friendly plain text. |
| `limit` | `20` | Number of posts to include. Maximum is `50`. |
| `links` | `1` | When enabled, visible links become `text (url)`. Use `links=0` to keep only visible text. |
| `source` | `0` | Use `source=1` to append the original Telegram post URL to the item text. |

## Run locally

```bash
go build -o rss-vk-proxy-go ./main.go
RSSVK_CACHE_DIR=./cache ./rss-vk-proxy-go 127.0.0.1:18766
```

Healthcheck:

```bash
curl http://127.0.0.1:18766/healthz
```

## Run through a proxy

Telegram may be unavailable from some networks. The service uses Go's standard proxy environment variables:

```bash
RSSVK_CACHE_DIR=./cache \
HTTPS_PROXY=http://127.0.0.1:3128 \
HTTP_PROXY=http://127.0.0.1:3128 \
./rss-vk-proxy-go 127.0.0.1:18766
```

## systemd example

A sample unit is available in:

```text
contrib/rss-vk-proxy.service.example
```

Adjust `WorkingDirectory`, `ExecStart`, proxy variables, and `RSSVK_CACHE_DIR` for your host.

## nginx example

```nginx
location /rssvk/ {
    proxy_pass http://127.0.0.1:18766;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

Then use:

```text
https://example.com/rssvk/telegram/channel/durov?mode=vk
```

## Cache and privacy notes

Do **not** commit `cache/`.

Telegram public HTML can contain temporary CDN URLs for media. Those URLs may include token-like query parameters, so `cache/` is intentionally ignored by git. The compiled binary is ignored too.

## Limitations

- Only public Telegram channel pages are supported.
- This is an HTML parser for Telegram's public web view, so Telegram markup changes can require parser updates.
- Media support is intentionally conservative; the feed primarily targets readable text import.

## License

MIT
