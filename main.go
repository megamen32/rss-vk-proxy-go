package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Item struct {
	ID     int
	Link   string
	Date   time.Time
	Text   string
	Title  string
	Images []string
}
type Server struct {
	bind, cacheDir string
	client         *http.Client
}

var (
	reWrap        = regexp.MustCompile(`(?s)<div class="tgme_widget_message_wrap\b`)
	rePost        = regexp.MustCompile(`data-post="([^"]+)/(\d+)"`)
	reDate        = regexp.MustCompile(`<time\s+datetime="([^"]+)"`)
	reText        = regexp.MustCompile(`(?s)<div class="tgme_widget_message_text\s+js-message_text"[^>]*>`)
	rePhoto       = regexp.MustCompile(`(?is)<a[^>]+tgme_widget_message_photo_wrap[^>]+background-image:url\('([^']+)'\)`)
	reA           = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
	reHref        = regexp.MustCompile(`(?is)\bhref="([^"]+)"`)
	reScript      = regexp.MustCompile(`(?is)<script\b.*?</script>`)
	reBlockquote  = regexp.MustCompile(`(?is)<blockquote\b.*?</blockquote>`)
	reLinkPreview = regexp.MustCompile(`(?is)<a\b[^>]*tgme_widget_message_link_preview[^>]*>.*?</a>`)
	reStyle       = regexp.MustCompile(`(?is)<style\b.*?</style>`)
	reTags        = regexp.MustCompile(`(?is)<[^>]+>`)
	reSpaces      = regexp.MustCompile(`[ \t\r\f\v]+`)
	reManyNL      = regexp.MustCompile(`\n{4,}`)
)

func main() {
	bind := "127.0.0.1:18766"
	if len(os.Args) > 1 && os.Args[1] != "" {
		bind = os.Args[1]
	}
	cacheDir := os.Getenv("RSSVK_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "cache"
	}
	_ = os.MkdirAll(cacheDir, 0755)
	s := &Server{bind: bind, cacheDir: cacheDir, client: &http.Client{Timeout: 35 * time.Second}}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/rssvk/telegram/channel/", s.feed)
	log.Printf("rss-vk-proxy-go listening on %s", bind)
	log.Fatal(http.ListenAndServe(bind, mux))
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok\n") }

func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimPrefix(r.URL.Path, "/rssvk/telegram/channel/")
	channel = strings.Trim(channel, "/")
	if channel == "" || strings.Contains(channel, "..") {
		http.Error(w, "bad channel", 400)
		return
	}
	q := r.URL.Query()
	limit := parseInt(q.Get("limit"), 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}
	appendLinks := q.Get("links") != "0"
	source := q.Get("source") == "1"
	items, err := s.fetchChannel(channel, appendLinks, source)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID > items[j].ID })
	if len(items) > limit {
		items = items[:limit]
	}
	rss := buildRSS(channel, items)
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	_, _ = w.Write(rss)
}

func parseInt(s string, d int) int {
	if s == "" {
		return d
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return d
	}
	return n
}

func (s *Server) fetchChannel(channel string, appendLinks bool, source bool) ([]Item, error) {
	u := "https://t.me/s/" + url.PathEscape(channel)
	body, err := s.fetchURL(u)
	if err != nil {
		return nil, err
	}
	return parseTelegram(channel, string(body), appendLinks, source), nil
}

func (s *Server) fetchURL(u string) ([]byte, error) {
	keyBytes := sha256.Sum256([]byte(u))
	key := hex.EncodeToString(keyBytes[:])
	cache := filepath.Join(s.cacheDir, key+".html")
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 RSS VK Direct Go")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru,en;q=0.8")
	resp, err := s.client.Do(req)
	if err == nil && resp != nil && resp.Body != nil {
		defer resp.Body.Close()
		b, readErr := io.ReadAll(resp.Body)
		if readErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && bytes.Contains(b, []byte("tgme_widget_message")) {
			_ = os.WriteFile(cache, b, 0644)
			return b, nil
		}
		if readErr != nil {
			err = readErr
		} else {
			err = fmt.Errorf("telegram status %d", resp.StatusCode)
		}
	}
	if b, readErr := os.ReadFile(cache); readErr == nil && bytes.Contains(b, []byte("tgme_widget_message")) {
		return b, nil
	}
	if err == nil {
		err = fmt.Errorf("telegram fetch failed")
	}
	return nil, err
}

func parseTelegram(channel, h string, appendLinks bool, source bool) []Item {
	idxs := reWrap.FindAllStringIndex(h, -1)
	out := make([]Item, 0, len(idxs))
	for i, idx := range idxs {
		start := idx[0]
		end := len(h)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		block := h[start:end]
		pm := rePost.FindStringSubmatch(block)
		if pm == nil {
			continue
		}
		id, _ := strconv.Atoi(pm[2])
		link := "https://t.me/" + pm[1] + "/" + pm[2]
		dt := time.Now().UTC()
		if dm := reDate.FindStringSubmatch(block); dm != nil {
			if t, err := time.Parse(time.RFC3339, dm[1]); err == nil {
				dt = t
			}
		}
		textHTML := ""
		if tm := reText.FindStringIndex(block); tm != nil {
			textHTML = untilClosingDiv(block[tm[1]:])
		}
		imgs := extractImages(block)
		text := cleanText(textHTML, appendLinks)
		// Prefer the original Telegram text over RSSHub's sometimes inconsistent
		// quote/reply truncation. Keep blockquotes as normal text and only normalize
		// excessive paragraph gaps for VK.
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
		if text == "" && len(imgs) > 0 {
			text = "🖼"
		}
		if source && text != "" {
			text += "\n\nИсточник: " + link
		}
		title := titleFromText(text)
		if title == "" {
			title = fmt.Sprintf("Telegram post %d", id)
		}
		out = append(out, Item{ID: id, Link: link, Date: dt, Text: text, Title: title, Images: imgs})
	}
	return out
}

func untilClosingDiv(s string) string {
	if i := strings.Index(strings.ToLower(s), "</div>"); i >= 0 {
		return s[:i]
	}
	return s
}

func extractImages(block string) []string {
	seen := map[string]bool{}
	var imgs []string
	for _, m := range rePhoto.FindAllStringSubmatch(block, -1) {
		u := html.UnescapeString(m[1])
		if strings.HasPrefix(u, "//") {
			u = "https:" + u
		}
		if u != "" && !seen[u] {
			seen[u] = true
			imgs = append(imgs, u)
		}
	}
	return imgs
}

func cleanText(s string, appendLinks bool) string {
	s = reScript.ReplaceAllString(s, "")
	s = reStyle.ReplaceAllString(s, "")
	s = reLinkPreview.ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`(?i)</?(p|div|section|article|pre|blockquote)[^>]*>`).ReplaceAllString(s, "\n\n")
	s = reA.ReplaceAllStringFunc(s, func(a string) string {
		m := reA.FindStringSubmatch(a)
		if m == nil {
			return a
		}
		href := ""
		if hm := reHref.FindStringSubmatch(m[1]); hm != nil {
			href = html.UnescapeString(hm[1])
		}
		visible := stripTags(m[2])
		visible = html.UnescapeString(visible)
		visible = normalizeText(visible)
		if appendLinks && href != "" && strings.HasPrefix(href, "http") && !strings.Contains(visible, href) {
			if visible != "" {
				return visible + " (" + href + ")"
			}
			return href
		}
		return visible
	})
	s = stripTags(s)
	s = html.UnescapeString(s)
	return normalizeText(s)
}
func stripTags(s string) string { return reTags.ReplaceAllString(s, "") }
func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = reSpaces.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	s = strings.Join(lines, "\n")
	s = reManyNL.ReplaceAllString(s, "\n\n\n")
	return strings.TrimSpace(s)
}
func titleFromText(s string) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			r := []rune(l)
			if len(r) > 140 {
				l = string(r[:140])
			}
			return l
		}
	}
	return ""
}

func buildRSS(channel string, items []Item) []byte {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:media="http://search.yahoo.com/mrss/"><channel>`)
	writeEl(&b, "title", channel+" - Telegram Channel — VK friendly")
	writeEl(&b, "link", "https://t.me/"+channel)
	writeEl(&b, "description", "VK-friendly RSS generated directly from Telegram public channel")
	writeEl(&b, "language", "ru")
	writeEl(&b, "lastBuildDate", time.Now().UTC().Format(time.RFC1123Z))
	for _, it := range items {
		b.WriteString("<item>")
		writeEl(&b, "title", it.Title)
		writeEl(&b, "link", it.Link)
		writeEl(&b, "guid", it.Link)
		writeEl(&b, "pubDate", it.Date.UTC().Format(time.RFC1123Z))
		writeEl(&b, "description", it.Text)
		writeEl(&b, "content:encoded", it.Text)
		for _, img := range it.Images {
			b.WriteString(`<enclosure url="` + xmlEsc(img) + `" type="image/jpeg" />`)
			b.WriteString(`<media:content url="` + xmlEsc(img) + `" medium="image" />`)
		}
		b.WriteString("</item>")
	}
	b.WriteString("</channel></rss>")
	return b.Bytes()
}
func writeEl(b *bytes.Buffer, name, val string) {
	b.WriteString("<" + name + ">")
	xml.EscapeText(b, []byte(val))
	b.WriteString("</" + name + ">")
}
func xmlEsc(s string) string { var b bytes.Buffer; xml.EscapeText(&b, []byte(s)); return b.String() }
