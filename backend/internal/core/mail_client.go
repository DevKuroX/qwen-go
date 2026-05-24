package core

import (
	"crypto/rand"
	"encoding/base64"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type GuerrillaMailClient struct {
	emailAddr string
	apiURL    string
	sidToken  string
	client    *http.Client
}

func NewGuerrillaMailClient() *GuerrillaMailClient {
	return &GuerrillaMailClient{
		apiURL: "https://api.guerrillamail.com/ajax.php",
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (g *GuerrillaMailClient) CreateAddress() (string, error) {
	resp, err := g.client.Get(g.apiURL + "?f=get_email_address")
	if err != nil {
		return "", fmt.Errorf("guerrilla get_email failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		EmailAddr string `json:"email_addr"`
		SIDToken  string `json:"sid_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("guerrilla parse failed: %w", err)
	}

	g.emailAddr = result.EmailAddr
	g.sidToken = result.SIDToken
	return result.EmailAddr, nil
}

func (g *GuerrillaMailClient) PollForActivationLink(ctx context.Context, maxPolls int) (string, error) {
	for i := 0; i < maxPolls; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}

		u := fmt.Sprintf("%s?f=check_email&seq=0&sid_token=%s", g.apiURL, url.QueryEscape(g.sidToken))
		resp, err := g.client.Get(u)
		if err != nil {
			continue
		}

		var result struct {
			List []struct {
				MailFrom string `json:"mail_from"`
				MailBody string `json:"mail_body"`
			} `json:"list"`
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		for _, email := range result.List {
			if link := extractActivationLink(email.MailBody); link != "" {
				return link, nil
			}
		}
	}
	return "", fmt.Errorf("activation email not found after %d polls", maxPolls)
}

type TempMailClient struct {
	client    *http.Client
	baseURL   string
	apiKey    string
	domain    string
	emailAddr string
	authToken string
}

func NewTempMailClient(domain, apiKey string) *TempMailClient {
	return &TempMailClient{
		client:  &http.Client{Timeout: 15 * time.Second},
		baseURL: fmt.Sprintf("https://%s", domain),
		domain:  domain,
		apiKey:  apiKey,
	}
}

func (t *TempMailClient) CreateAddress() (string, error) {
	u := fmt.Sprintf("%s/api/new", t.baseURL)
	req, err := http.NewRequest(http.MethodPost, u, nil)
	if err != nil {
		return "", fmt.Errorf("tempmail request build failed: %w", err)
	}
	req.Header.Set("X-Admin-Password", t.apiKey)
	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tempmail create failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Address string `json:"address"`
		JWT     string `json:"jwt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("tempmail parse failed: %w", err)
	}

	t.emailAddr = result.Address
	t.authToken = result.JWT
	return result.Address, nil
}

func (t *TempMailClient) PollForActivationLink(ctx context.Context, maxPolls int) (string, error) {
	for i := 0; i < maxPolls; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}

		u := fmt.Sprintf("%s/api/messages", t.baseURL)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+t.authToken)
		resp, err := t.client.Do(req)
		if err != nil {
			continue
		}

		var messages []struct {
			ID      string `json:"id"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err := json.Unmarshal(body, &messages); err != nil {
			continue
		}

		for _, msg := range messages {
			if link := extractActivationLink(msg.Body); link != "" {
				return link, nil
			}
		}
	}
	return "", fmt.Errorf("activation email not found after %d polls", maxPolls)
}

type MoeMailClient struct {
	client    *http.Client
	baseURL   string
	apiKey    string
	emailID   string
	emailAddr string
}

func NewMoeMailClient(domain, apiKey string) *MoeMailClient {
	return &MoeMailClient{client: &http.Client{Timeout: 15 * time.Second}, baseURL: strings.TrimRight(domain, "/"), apiKey: apiKey}
}

func (m *MoeMailClient) CreateAddress() (string, error) {
	req, err := http.NewRequest(http.MethodPost, m.baseURL+"/api/address", nil)
	if err != nil {
		return "", fmt.Errorf("moemail request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("moemail create failed: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		Address string `json:"address"`
		ID      string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("moemail parse failed: %w", err)
	}
	m.emailAddr = result.Address
	m.emailID = result.ID
	if m.emailAddr == "" {
		return "", fmt.Errorf("moemail returned empty address")
	}
	return m.emailAddr, nil
}

func (m *MoeMailClient) PollForActivationLink(ctx context.Context, maxPolls int) (string, error) {
	for i := 0; i < maxPolls; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/messages/%s", m.baseURL, m.emailID), nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
		resp, err := m.client.Do(req)
		if err != nil {
			continue
		}
		var messages []struct{ Subject, HTML, Text string }
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err := json.Unmarshal(body, &messages); err != nil {
			continue
		}
		for _, msg := range messages {
			if link := extractActivationLink(msg.HTML + "\n" + msg.Text); link != "" {
				return link, nil
			}
		}
	}
	return "", fmt.Errorf("activation email not found after %d polls", maxPolls)
}

type LocalMailClient struct {
	client    *http.Client
	baseURL   string
	domain    string
	emailAddr string
}

func NewLocalMailClient(baseURL, domain string) *LocalMailClient {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if domain == "" {
		domain = "snapsave.my.id"
	}
	return &LocalMailClient{client: &http.Client{Timeout: 15 * time.Second}, baseURL: strings.TrimRight(baseURL, "/"), domain: domain}
}

func (l *LocalMailClient) CreateAddress() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("localmail random failed: %w", err)
	}
	name := strings.ToLower(base64.RawURLEncoding.EncodeToString(b))
	l.emailAddr = name + "@" + l.domain
	return l.emailAddr, nil
}

func (l *LocalMailClient) PollForActivationLink(ctx context.Context, maxPolls int) (string, error) {
	for i := 0; i < maxPolls; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}
		resp, err := l.client.Get(fmt.Sprintf("%s/api/email/%s", l.baseURL, l.emailAddr))
		if err != nil {
			continue
		}
		var result struct{ HTML, Text string }
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}
		if link := extractActivationLink(result.HTML + "\n" + result.Text); link != "" {
			return link, nil
		}
	}
	return "", fmt.Errorf("activation email not found after %d polls", maxPolls)
}

type GPTMailClient struct {
	client *http.Client
	apiURL string
	email  string
}

func NewGPTMailClient() *GPTMailClient {
	return &GPTMailClient{
		client: &http.Client{Timeout: 15 * time.Second},
		apiURL: "https://mail.chatgpt.org.uk",
	}
}

func (g *GPTMailClient) CreateAddress() (string, error) {
	u := fmt.Sprintf("%s/api/v1/mailbox", g.apiURL)
	resp, err := g.client.Post(u, "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("gptmail create failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("gptmail parse failed: %w", err)
	}

	g.email = result.Address
	return result.Address, nil
}

func (g *GPTMailClient) PollForActivationLink(ctx context.Context, maxPolls int) (string, error) {
	for i := 0; i < maxPolls; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}

		u := fmt.Sprintf("%s/api/v1/mailbox/%s/messages", g.apiURL, g.email)
		resp, err := g.client.Get(u)
		if err != nil {
			continue
		}

		var messages []struct {
			From    string `json:"from"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err := json.Unmarshal(body, &messages); err != nil {
			continue
		}

		for _, msg := range messages {
			if link := extractActivationLink(msg.Body); link != "" {
				return link, nil
			}
		}
	}
	return "", fmt.Errorf("activation email not found after %d polls", maxPolls)
}

func extractActivationLink(body string) string {
	b64Match := regexp.MustCompile(`Content-Transfer-Encoding: base64\r?\n\r?\n([A-Za-z0-9+/=\r\n]+)`).FindStringSubmatch(body)
	if len(b64Match) == 2 {
		encoded := strings.ReplaceAll(strings.ReplaceAll(b64Match[1], "\r", ""), "\n", "")
		if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
			body = string(decoded)
		}
	}

	// Look for Qwen activation URL patterns
	markers := []string{
		"https://chat.qwen.ai/api/v1/auths/activate?",
		"https://chat.qwen.ai/api/v1/auths/verify?",
		"chat.qwen.ai/api/v1/auths/activate",
		"chat.qwen.ai/api/v1/auths/verify",
		"qwen.ai/activate",
		"qwen.ai/verify",
	}
	for _, marker := range markers {
		idx := strings.Index(body, marker)
		if idx == -1 {
			continue
		}
		start := idx
		end := idx + len(marker)
		for end < len(body) && body[end] != '"' && body[end] != '\'' && body[end] != ' ' && body[end] != '<' && body[end] != '>' {
			end++
		}
		link := body[start:end]
		if !strings.HasPrefix(link, "http") {
			link = "https://" + link
		}
		return link
	}
	return ""
}
