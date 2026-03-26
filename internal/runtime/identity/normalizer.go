package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

type Config struct {
	HeaderPriority    []string
	BodyPriority      []string
	TimeBucketMinutes int
	SessionTTLMinutes int
	Now               func() time.Time
}

type Input struct {
	Headers    http.Header
	Body       []byte
	Path       string
	RemoteAddr string
	APIKeyHash string
	UserAgent  string
}

type Result struct {
	SessionKey   string
	ThreadKey    string
	ClientFamily string
	Source       string
	SessionIDRaw string
	ThreadIDRaw  string
}

func DefaultConfig() Config {
	return Config{
		HeaderPriority:    []string{"x-session-id", "x-thread-id", "x-conversation-id", "x-client-id", "x-user-id"},
		BodyPriority:      []string{"conversation_id", "thread_id", "session_id", "metadata.conversation_id", "metadata.thread_id", "metadata.session_id"},
		SessionTTLMinutes: 120,
		Now:               time.Now,
	}
}

type cacheEntry struct {
	result    Result
	expiresAt time.Time
}

// StatefulNormalizer adds TTL caching on top of Normalize() to keep session identity
// stable across short windows, especially for fallback-derived identities.
type StatefulNormalizer struct {
	cfg   Config
	mu    sync.Mutex
	cache map[string]cacheEntry
}

func NewStatefulNormalizer(cfg Config) *StatefulNormalizer {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.TimeBucketMinutes <= 0 {
		cfg.TimeBucketMinutes = 15
	}
	if cfg.SessionTTLMinutes <= 0 {
		cfg.SessionTTLMinutes = 120
	}
	return &StatefulNormalizer{cfg: cfg, cache: map[string]cacheEntry{}}
}

func (n *StatefulNormalizer) Resolve(in Input) Result {
	if n == nil {
		cfg := DefaultConfig()
		return Normalize(cfg, in)
	}
	now := n.cfg.Now()
	family := inferClientFamily(in.UserAgent, in.Headers)
	sessionRaw, source := pickSessionID(n.cfg, in.Headers, in.Body)
	threadRaw := pickThreadID(n.cfg, in.Headers, in.Body)
	if strings.TrimSpace(threadRaw) == "" {
		threadRaw = sessionRaw
	}
	if strings.TrimSpace(sessionRaw) == "" {
		source = "fallback"
		sessionRaw = fallbackIdentity(n.cfg, in)
		if strings.TrimSpace(threadRaw) == "" {
			threadRaw = sessionRaw
		}
	}
	cacheKey := identityCacheKey(family, source, sessionRaw, threadRaw, in)

	n.mu.Lock()
	if entry, ok := n.cache[cacheKey]; ok {
		if entry.expiresAt.After(now) {
			res := entry.result
			n.mu.Unlock()
			return res
		}
		delete(n.cache, cacheKey)
	}
	res := Result{
		SessionKey:   "nm_sess:" + family + ":" + shortHash(family+"|"+sessionRaw),
		ThreadKey:    "nm_thread:" + family + ":" + shortHash(family+"|"+threadRaw),
		ClientFamily: family,
		Source:       source,
		SessionIDRaw: sessionRaw,
		ThreadIDRaw:  threadRaw,
	}
	n.cache[cacheKey] = cacheEntry{
		result:    res,
		expiresAt: now.Add(time.Duration(n.cfg.SessionTTLMinutes) * time.Minute),
	}
	if len(n.cache) > 4096 {
		for k, v := range n.cache {
			if !v.expiresAt.After(now) {
				delete(n.cache, k)
			}
		}
	}
	n.mu.Unlock()
	return res
}

func identityCacheKey(family, source, sessionRaw, threadRaw string, in Input) string {
	if source != "fallback" {
		return "explicit|" + family + "|" + strings.ToLower(strings.TrimSpace(sessionRaw)) + "|" + strings.ToLower(strings.TrimSpace(threadRaw))
	}
	apiKeyHash := strings.TrimSpace(in.APIKeyHash)
	if apiKeyHash == "" {
		apiKeyHash = "-"
	}
	return "fallback|" + family + "|" + apiKeyHash + "|" + normalizeUASignature(in.UserAgent) + "|" + maskIP(in.RemoteAddr) + "|" + normalizePathGroup(in.Path)
}

func Normalize(cfg Config, in Input) Result {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.TimeBucketMinutes <= 0 {
		cfg.TimeBucketMinutes = 15
	}
	family := inferClientFamily(in.UserAgent, in.Headers)
	sessionRaw, source := pickSessionID(cfg, in.Headers, in.Body)
	threadRaw := pickThreadID(cfg, in.Headers, in.Body)
	if strings.TrimSpace(threadRaw) == "" {
		threadRaw = sessionRaw
	}
	if strings.TrimSpace(sessionRaw) == "" {
		source = "fallback"
		sessionRaw = fallbackIdentity(cfg, in)
		if strings.TrimSpace(threadRaw) == "" {
			threadRaw = sessionRaw
		}
	}

	sessionKey := "nm_sess:" + family + ":" + shortHash(family+"|"+sessionRaw)
	threadKey := "nm_thread:" + family + ":" + shortHash(family+"|"+threadRaw)
	return Result{
		SessionKey:   sessionKey,
		ThreadKey:    threadKey,
		ClientFamily: family,
		Source:       source,
		SessionIDRaw: sessionRaw,
		ThreadIDRaw:  threadRaw,
	}
}

func pickSessionID(cfg Config, headers http.Header, body []byte) (string, string) {
	for i := range cfg.HeaderPriority {
		name := strings.ToLower(strings.TrimSpace(cfg.HeaderPriority[i]))
		if name == "" {
			continue
		}
		if v := strings.TrimSpace(headers.Get(name)); v != "" {
			return v, "header:" + name
		}
	}
	for i := range cfg.BodyPriority {
		path := strings.TrimSpace(cfg.BodyPriority[i])
		if path == "" {
			continue
		}
		if v := strings.TrimSpace(gjson.GetBytes(body, path).String()); v != "" {
			return v, "body:" + path
		}
	}
	return "", "none"
}

func pickThreadID(cfg Config, headers http.Header, body []byte) string {
	if v := strings.TrimSpace(headers.Get("x-thread-id")); v != "" {
		return v
	}
	if v := strings.TrimSpace(headers.Get("x-conversation-id")); v != "" {
		return v
	}
	if v := strings.TrimSpace(gjson.GetBytes(body, "thread_id").String()); v != "" {
		return v
	}
	if v := strings.TrimSpace(gjson.GetBytes(body, "conversation_id").String()); v != "" {
		return v
	}
	if v := strings.TrimSpace(gjson.GetBytes(body, "metadata.thread_id").String()); v != "" {
		return v
	}
	if v := strings.TrimSpace(gjson.GetBytes(body, "metadata.conversation_id").String()); v != "" {
		return v
	}
	return ""
}

func fallbackIdentity(cfg Config, in Input) string {
	bucketMinutes := cfg.TimeBucketMinutes
	bucket := cfg.Now().UTC().Unix() / int64(bucketMinutes*60)
	apiKeyHash := strings.TrimSpace(in.APIKeyHash)
	if apiKeyHash == "" {
		apiKeyHash = "-"
	}
	ua := normalizeUASignature(in.UserAgent)
	ip := maskIP(in.RemoteAddr)
	pathGroup := normalizePathGroup(in.Path)
	compound := apiKeyHash + "|" + ua + "|" + ip + "|" + pathGroup + "|" + strconvI64(bucket)
	return shortHash(compound)
}

func inferClientFamily(userAgent string, headers http.Header) string {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		ua = strings.ToLower(strings.TrimSpace(headers.Get("user-agent")))
	}
	switch {
	case strings.Contains(ua, "amp"):
		return "ampcode"
	case strings.Contains(ua, "claude"):
		return "claude_code"
	case strings.Contains(ua, "vscode"):
		return "vscode"
	case strings.Contains(ua, "openclaw"):
		return "openclaw"
	case strings.Contains(ua, "opencode"):
		return "opencode"
	default:
		return "generic"
	}
}

func normalizeUASignature(ua string) string {
	ua = strings.TrimSpace(strings.ToLower(ua))
	if ua == "" {
		return "-"
	}
	if len(ua) > 120 {
		ua = ua[:120]
	}
	return ua
}

func normalizePathGroup(path string) string {
	p := strings.TrimSpace(strings.ToLower(path))
	if p == "" {
		return "/"
	}
	if strings.HasPrefix(p, "/api/provider/anthropic") {
		return "/api/provider/anthropic"
	}
	if strings.HasPrefix(p, "/api/provider/openai") {
		return "/api/provider/openai"
	}
	if strings.HasPrefix(p, "/v1/") {
		return "/v1"
	}
	return p
}

func maskIP(remoteAddr string) string {
	host := strings.TrimSpace(remoteAddr)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "-"
	}
	if v4 := ip.To4(); v4 != nil {
		return strconvI(int(v4[0])) + "." + strconvI(int(v4[1])) + "." + strconvI(int(v4[2])) + ".0/24"
	}
	v6 := ip.To16()
	if v6 == nil {
		return "-"
	}
	parts := []string{
		hex.EncodeToString(v6[0:2]),
		hex.EncodeToString(v6[2:4]),
		hex.EncodeToString(v6[4:6]),
		hex.EncodeToString(v6[6:8]),
	}
	return strings.Join(parts, ":") + "::/64"
}

func shortHash(v string) string {
	s := sha256.Sum256([]byte(v))
	return hex.EncodeToString(s[:])[:24]
}

func strconvI(v int) string {
	return strconvI64(int64(v))
}

func strconvI64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + (v % 10))
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
