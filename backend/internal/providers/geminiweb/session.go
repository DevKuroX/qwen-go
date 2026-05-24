package geminiweb

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type httpClient struct {
	client  *http.Client
	cookies map[string]string
}

func newHTTPClient(proxy string) *httpClient {
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{}
	if proxy != "" {
		if proxyURL, err := url.Parse(proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &httpClient{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
			Jar:       jar,
		},
		cookies: make(map[string]string),
	}
}

func (c *httpClient) setCookies(cookies map[string]string) {
	for k, v := range cookies {
		c.cookies[k] = v
	}
}

func (c *httpClient) do(req *http.Request) (*http.Response, error) {
	for name, value := range c.cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value, Domain: ".google.com"})
	}
	return c.client.Do(req)
}

var (
	snlM0eRe     = regexp.MustCompile(`"SNlM0e":"([^"]+)"`)
	buildLabelRe = regexp.MustCompile(`"buildLabel":"([^"]+)"`)
	sessionIDRe  = regexp.MustCompile(`"sessionId":"([^"]+)"`)
)

func newSession(psid, psidts, proxy string, modelLookup func(string) modelMeta) *session {
	s := &session{
		Secure1PSID:   psid,
		Secure1PSIDTS: psidts,
		Language:      "en",
		Proxy:         proxy,
		reqCounter:    10000,
		modelLookup:   modelLookup,
	}
	s.client = newHTTPClient(proxy)
	s.client.setCookies(map[string]string{
		"__Secure-1PSID":   psid,
		"__Secure-1PSIDTS": psidts,
	})
	return s
}

// Init bootstraps the session: preflight google.com, fetch gemini.google.com
// /app, and scrape SNlM0e + buildLabel + sessionId from the HTML.
func (s *session) Init() error {
	req, _ := http.NewRequest("GET", endpointGoogle, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	resp, err := s.client.do(req)
	if err != nil {
		return fmt.Errorf("preflight failed: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	req, _ = http.NewRequest("GET", endpointInit, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Origin", "https://gemini.google.com")
	req.Header.Set("Referer", "https://gemini.google.com/")

	resp, err = s.client.do(req)
	if err != nil {
		return fmt.Errorf("init request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read init response: %w", err)
	}
	html := string(body)

	matches := snlM0eRe.FindStringSubmatch(html)
	if len(matches) < 2 {
		return fmt.Errorf("failed to extract SNlM0e token (cookies may be invalid or expired)")
	}
	s.AccessToken = matches[1]

	if m := buildLabelRe.FindStringSubmatch(html); len(m) >= 2 {
		s.BuildLabel = m[1]
	}
	if m := sessionIDRe.FindStringSubmatch(html); len(m) >= 2 {
		s.SessionID = m[1]
	}

	s.lastRefresh = time.Now()
	return nil
}

func (s *session) RefreshAccessToken() error {
	if err := s.rotateCookies(); err != nil {
		return err
	}
	return s.Init()
}

func (s *session) rotateCookies() error {
	data := fmt.Sprintf(`[null,null,[[null,null,null,null,null,null,null,null,"%s"]]]`, s.Secure1PSIDTS)

	req, _ := http.NewRequest("POST", endpointRotate, strings.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://accounts.google.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := s.client.do(req)
	if err != nil {
		return fmt.Errorf("cookie rotation failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("cookie rotation returned %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	parts := strings.Split(string(body), `"`)
	for i, p := range parts {
		if strings.HasPrefix(p, "sidts-") {
			s.Secure1PSIDTS = p
			s.client.setCookies(map[string]string{
				"__Secure-1PSID":   s.Secure1PSID,
				"__Secure-1PSIDTS": s.Secure1PSIDTS,
			})
			break
		}
		if i > 0 && strings.Contains(p, "sidts-") {
			s.Secure1PSIDTS = strings.TrimSuffix(p, `=`)
			s.client.setCookies(map[string]string{
				"__Secure-1PSID":   s.Secure1PSID,
				"__Secure-1PSIDTS": s.Secure1PSIDTS,
			})
			break
		}
	}

	return nil
}

func (s *session) IsTokenExpired(ttl time.Duration) bool {
	return time.Since(s.lastRefresh) > ttl
}

func (s *session) IsAuthenticated() bool {
	return s.AccessToken != ""
}

func (s *session) resolveModel(name string) modelMeta {
	if s.modelLookup != nil {
		if m := s.modelLookup(name); m.ModelID != "" {
			return m
		}
	}
	if m, ok := defaultModels[name]; ok {
		return m
	}
	return defaultModels["gemini-3-flash"]
}

// UploadImage POSTs raw bytes to content-push.googleapis.com/upload (the same
// endpoint gemini.google.com uses for file attachments). The response body is
// the bare upload URL — that string slots straight into file_data as
// [[url], filename]. Mirrors HanaokaYuzu/Gemini-API utils/upload_file.py.
func (s *session) UploadImage(data []byte, filename string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty image payload")
	}
	if filename == "" {
		filename = "input.png"
	}

	body := &bytes.Buffer{}
	mp := multipart.NewWriter(body)

	// Match the python lib's multipart shape: name="file", filename=…,
	// Content-Type derived from extension.
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	hdr.Set("Content-Type", mimeTypeFromName(filename))
	part, err := mp.CreatePart(hdr)
	if err != nil {
		return "", fmt.Errorf("multipart part: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("multipart write: %w", err)
	}
	if err := mp.Close(); err != nil {
		return "", fmt.Errorf("multipart close: %w", err)
	}

	req, err := http.NewRequest("POST", endpointUpload, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("X-Tenant-Id", "bard-storage")
	req.Header.Set("Push-ID", "feeds/mcudyrk2a4khkz")
	req.Header.Set("Origin", "https://gemini.google.com")
	req.Header.Set("Referer", "https://gemini.google.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := s.client.do(req)
	if err != nil {
		return "", fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("upload HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	urlStr := strings.TrimSpace(string(respBody))
	if urlStr == "" {
		return "", fmt.Errorf("upload returned empty body")
	}
	return urlStr, nil
}

func mimeTypeFromName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
