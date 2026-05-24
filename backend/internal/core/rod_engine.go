package core

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/qwenpi/qwenpi-go/internal/models"
	"go.uber.org/zap"
)

const (
	elementTimeout = 8 * time.Second
	pageHTMLMaxLen = 2000
)

type RodEngine struct {
	logger        *zap.Logger
	proxyProvider ProxyEnvProvider
	mu            chan struct{}
}

func NewRodEngine() *RodEngine {
	return &RodEngine{
		logger: zap.L(),
		mu:     make(chan struct{}, 1),
	}
}

func (e *RodEngine) SetProxyProvider(provider ProxyEnvProvider) {
	e.proxyProvider = provider
}

func (e *RodEngine) Name() string {
	return "rod"
}

func (e *RodEngine) Register(ctx context.Context, req models.RegistrationRequest, onLog func(string)) (*models.Account, error) {
	select {
	case e.mu <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, fmt.Errorf("rod engine busy, another registration in progress")
	}
	defer func() { <-e.mu }()

	onLog = orNoop(onLog)

	password, username := newPassword(), newUsername()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	mailClient, mailLabel, err := e.createMailClient(req.Provider)
	if err != nil {
		return nil, fmt.Errorf("mail client error: %w", err)
	}

	onLog(fmt.Sprintf("Creating email via %s...", mailLabel))
	emailAddr, err := mailClient.CreateAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to create email: %w", err)
	}
	onLog(fmt.Sprintf("Email created: %s", emailAddr))

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	onLog("Launching browser...")
	browser, err := e.launchBrowser(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}
	defer browser.Close()

	page := browser.MustPage()
	defer func() { _ = page.Close() }()

	if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: 1280, Height: 800}); err != nil {
		return nil, fmt.Errorf("set viewport failed: %w", err)
	}

	// Anti-detection + locale (same as Python playwright)
	_, _ = page.Eval(`Object.defineProperty(navigator, 'webdriver', { get: () => undefined })`)
	_, _ = page.Eval(`() => { Object.defineProperty(navigator, 'language', { get: () => 'zh-CN' }) }`)
	_, _ = page.Eval(`() => { Object.defineProperty(navigator, 'languages', { get: () => ['zh-CN', 'zh'] }) }`)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	onLog("Opening registration page...")
	if err := page.Navigate("https://chat.qwen.ai/auth?mode=register"); err != nil {
		return nil, fmt.Errorf("navigate failed: %w", err)
	}

	if err := page.WaitLoad(); err != nil {
		time.Sleep(3 * time.Second)
	}

	onLog("Waiting for page to render...")
	waitReady(page, ctx)
	time.Sleep(time.Duration(800+rand.Intn(400)) * time.Millisecond)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	onLog("Filling registration form...")

	if !tryFillShort(page, `input[placeholder*="名称"]`, username) &&
		!tryFillShort(page, `input[placeholder*="name"]`, username) &&
		!tryFillShort(page, `input[placeholder*="Name"]`, username) {
		onLog("Could not find username field. " + dumpPage(page))
	}
	sleepJitter(200, 200)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if !tryFillShort(page, `input[placeholder*="邮箱"]`, emailAddr) &&
		!tryFillShort(page, `input[placeholder*="mail"]`, emailAddr) &&
		!tryFillShort(page, `input[placeholder*="Mail"]`, emailAddr) &&
		!tryFillShort(page, `input[placeholder*="Email"]`, emailAddr) {
		onLog("Could not find email field. " + dumpPage(page))
	}
	sleepJitter(200, 200)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if els := findElements(page, `input[type="password"]`); len(els) > 0 {
		els[0].MustInput(password)
		sleepJitter(150, 150)
		if len(els) > 1 {
			els[1].MustInput(password)
			sleepJitter(150, 150)
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if els := findElements(page, `input[type="checkbox"]`); len(els) > 0 {
		els[0].MustClick()
		time.Sleep(200 * time.Millisecond)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	onLog("Clicking Create account...")
	if !clickButton(page, "创建账号") &&
		!clickButton(page, "Create account") &&
		!clickButton(page, "注册") &&
		!clickButton(page, "Sign Up") &&
		!clickButton(page, "Sign up") {
		if els := findElements(page, `button[type="submit"]`); len(els) > 0 {
			els[0].MustClick()
		} else {
			onLog("Page state: " + dumpPage(page))
			return nil, fmt.Errorf("cannot find create account button")
		}
	}

	onLog("Checking for captcha...")
	// Same logic as Python: wait up to 5s for captcha to appear
	captcha := detectCaptcha(page)
	if captcha {
		onLog("Captcha detected, discarding email")
		return nil, fmt.Errorf("captcha required, try different proxy")
	}

	onLog("No captcha detected, waiting for activation email...")
	time.Sleep(10 * time.Second)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cookies, err := page.Cookies(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get cookies: %w", err)
	}

	var cookieHeader strings.Builder
	browserToken := ""
	for i, c := range cookies {
		if i > 0 {
			cookieHeader.WriteString("; ")
		}
		cookieHeader.WriteString(c.Name)
		cookieHeader.WriteString("=")
		cookieHeader.WriteString(c.Value)
		if c.Name == "token" {
			browserToken = c.Value
		}
	}
	cookiesStr := cookieHeader.String()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	onLog("Polling for activation email...")
	pollCtx, pollCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer pollCancel()
	verifyURL, err := mailClient.PollForActivationLink(pollCtx, 24)
	if err != nil {
		return nil, fmt.Errorf("activation email timeout: %w", err)
	}

	onLog("Activation link received, completing registration...")

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if browserToken == "" {
		onLog("Visiting activation link...")
		_ = page.Navigate(verifyURL)
		time.Sleep(3 * time.Second)

		updatedCookies, _ := page.Cookies(nil)
		for _, c := range updatedCookies {
			if c.Name == "token" {
				browserToken = c.Value
				break
			}
		}
	}

	if browserToken == "" {
		onLog("Browser token missing, trying OAuth fallback...")
		fallbackToken, err := e.obtainOAuthToken(ctx, cookiesStr, emailAddr)
		if err == nil {
			browserToken = fallbackToken
		}
	}

	if browserToken == "" {
		return nil, fmt.Errorf("no token obtained after registration")
	}

	onLog(fmt.Sprintf("Registration successful: %s", emailAddr))

	return &models.Account{
		Email:    emailAddr,
		Password: password,
		Token:    browserToken,
		Cookies:  cookiesStr,
		Username: emailAddr[:strings.IndexByte(emailAddr, '@')],
		Status:   "VALID",
		Valid:    true,
		CreatedAt: time.Now(),
	}, nil
}

func (e *RodEngine) obtainOAuthToken(ctx context.Context, cookieHeader, email string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	verifier, challenge, err := generatePKCEPair()
	if err != nil {
		return "", err
	}

	deviceValues := url.Values{}
	deviceValues.Set("client_id", "f0304373b74a44d2b584a3fb70ca9e56")
	deviceValues.Set("scope", "openid profile email model.completion")
	deviceValues.Set("code_challenge", challenge)
	deviceValues.Set("code_challenge_method", "S256")
	deviceReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chat.qwen.ai/api/v1/oauth2/device/code", strings.NewReader(deviceValues.Encode()))
	if err != nil {
		return "", err
	}
	deviceReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	deviceResp, err := client.Do(deviceReq)
	if err != nil {
		return "", err
	}
	defer deviceResp.Body.Close()
	if deviceResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("device code request failed: %d", deviceResp.StatusCode)
	}
	var deviceData struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}
	if err := json.NewDecoder(deviceResp.Body).Decode(&deviceData); err != nil {
		return "", err
	}

	if deviceData.UserCode != "" && cookieHeader != "" {
		authorizeBody, _ := json.Marshal(map[string]interface{}{"approved": true, "user_code": deviceData.UserCode})
		authorizeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chat.qwen.ai/api/v2/oauth2/authorize", strings.NewReader(string(authorizeBody)))
		if err == nil {
			authorizeReq.Header.Set("Content-Type", "application/json")
			authorizeReq.Header.Set("Cookie", cookieHeader)
			if resp, err := client.Do(authorizeReq); err == nil {
				resp.Body.Close()
			}
		}
	}

	for attempt := 0; attempt < 12; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}

		tokenValues := url.Values{}
		tokenValues.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		tokenValues.Set("client_id", "f0304373b74a44d2b584a3fb70ca9e56")
		tokenValues.Set("device_code", deviceData.DeviceCode)
		tokenValues.Set("code_verifier", verifier)
		tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chat.qwen.ai/api/v1/oauth2/token", strings.NewReader(tokenValues.Encode()))
		if err != nil {
			return "", err
		}
		tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		tokenResp, err := client.Do(tokenReq)
		if err != nil {
			continue
		}
		if tokenResp.StatusCode == http.StatusOK {
			var tokenData struct {
				AccessToken string `json:"access_token"`
			}
			err = json.NewDecoder(tokenResp.Body).Decode(&tokenData)
			tokenResp.Body.Close()
			if err == nil && tokenData.AccessToken != "" {
				e.logger.Info("oauth fallback succeeded", zap.String("email", email))
				return tokenData.AccessToken, nil
			}
			continue
		}
		var errData struct{ Error string `json:"error"` }
		_ = json.NewDecoder(tokenResp.Body).Decode(&errData)
		tokenResp.Body.Close()
		if errData.Error == "authorization_pending" || errData.Error == "slow_down" || tokenResp.StatusCode == http.StatusGatewayTimeout {
			continue
		}
		break
	}
	return "", fmt.Errorf("oauth fallback failed for %s", email)
}

func generatePKCEPair() (string, string, error) {
	b := make([]byte, 32)
	if _, err := crand.Read(b); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// ── helpers ──────────────────────────────────────────────

func orNoop(fn func(string)) func(string) {
	if fn == nil {
		return func(string) {}
	}
	return fn
}

func sleepJitter(base, jitter int) {
	time.Sleep(time.Duration(base+rand.Intn(jitter)) * time.Millisecond)
}

// tryFillShort tries to find and fill an input. Returns true on success.
// Uses a short timeout so it won't block 120s waiting for a missing element.
func tryFillShort(page *rod.Page, selector, value string) bool {
	el, err := page.Timeout(elementTimeout).Element(selector)
	if err != nil {
		return false
	}
	return el.Input(value) == nil
}

// findElements returns matching elements with a short timeout (never blocks long).
func findElements(page *rod.Page, selector string) []*rod.Element {
	els, err := page.Timeout(elementTimeout).Elements(selector)
	if err != nil {
		return nil
	}
	return els
}

func clickButton(page *rod.Page, text string) bool {
	btn, err := page.Timeout(elementTimeout).Search(fmt.Sprintf(`button:has-text("%s")`, text))
	if err != nil {
		return false
	}
	if btn.First == nil {
		return false
	}
	btn.First.MustClick()
	return true
}

func waitReady(page *rod.Page, ctx context.Context) {
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for waitCtx.Err() == nil {
		done, err := page.Eval(`() => document.querySelector('input') !== null || document.readyState === 'complete'`)
		if err == nil && done.Value.Bool() {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func dumpPage(page *rod.Page) string {
	info, err := page.Info()
	if err != nil {
		return fmt.Sprintf("page info error: %v", err)
	}

	var parts []string
	if info.URL != "" {
		parts = append(parts, fmt.Sprintf("URL: %s", info.URL))
	}
	if info.Title != "" {
		parts = append(parts, fmt.Sprintf("Title: %s", info.Title))
	}

	// Get body inner HTML (avoids huge <style> blocks in <head>)
	bodyEl, bodyErr := page.Timeout(elementTimeout).Eval(`() => document.body ? document.body.innerHTML.substring(0, ` + fmt.Sprintf("%d", pageHTMLMaxLen) + `) : '(no body)'`)
	if bodyErr != nil {
		parts = append(parts, fmt.Sprintf("Body error: %v", bodyErr))
	} else if bodyEl != nil {
		bodyHTML := bodyEl.Value.Str()
		parts = append(parts, fmt.Sprintf("Body(%d): %s", len(bodyHTML), bodyHTML))
	}

	elemCount, _ := page.Timeout(elementTimeout).Eval(`() => document.querySelectorAll('*').length`)
	if elemCount != nil {
		parts = append(parts, fmt.Sprintf("DOM nodes: %d", elemCount.Value.Int()))
	}

	return strings.Join(parts, " | ")
}

func (e *RodEngine) createMailClient(provider string) (MailProvider, string, error) {
	switch provider {
	case "guerrilla":
		return NewGuerrillaMailClient(), "GuerrillaMail", nil
	case "gptmail", "default":
		return NewGPTMailClient(), "GPTMail", nil
	case "tempmail":
		if GlobalSettingsManager != nil {
			settings := GlobalSettingsManager.Get()
			if settings.TempMailDomain != "" && settings.TempMailKey != "" {
				return NewTempMailClient(settings.TempMailDomain, settings.TempMailKey), "TempMail", nil
			}
		}
		return nil, "", fmt.Errorf("tempmail configured but missing tempmail_domain/tempmail_key settings")
	case "moemail":
		if GlobalSettingsManager != nil {
			settings := GlobalSettingsManager.Get()
			if settings.MoeMailDomain != "" && settings.MoeMailKey != "" {
				return NewMoeMailClient(settings.MoeMailDomain, settings.MoeMailKey), "MoeMail", nil
			}
		}
		return nil, "", fmt.Errorf("moemail configured but missing moemail_domain/moemail_key settings")
	case "local":
		return NewLocalMailClient("", ""), "LocalMail", nil
	default:
		return nil, "", fmt.Errorf("unsupported mail provider: %s", provider)
	}
}

func (e *RodEngine) launchBrowser(ctx context.Context) (*rod.Browser, error) {
	headless := strings.ToLower(os.Getenv("ROD_HEADLESS")) != "false"

	l := launcher.New().
		Headless(headless).
		Set("disable-blink-features", "AutomationControlled").
		Set("no-sandbox", "").
		Set("disable-dev-shm-usage", "").
		Set("disable-gpu", "").
		Set("ignore-certificate-errors", "").
		Set("allow-insecure-localhost", "")

	if path, err := exec.LookPath("chromium"); err == nil {
		l = l.Bin(path)
	} else if path, err := exec.LookPath("chromium-browser"); err == nil {
		l = l.Bin(path)
	} else if path, err := exec.LookPath("google-chrome"); err == nil {
		l = l.Bin(path)
	} else if path, err := exec.LookPath("google-chrome-stable"); err == nil {
		l = l.Bin(path)
	}

	if e.proxyProvider != nil {
		enabled, proxyURL, _, _ := e.proxyProvider()
		if enabled && proxyURL != "" {
			parsedURL := proxyURL
			if !strings.HasPrefix(parsedURL, "socks5://") && !strings.HasPrefix(parsedURL, "http://") {
				parsedURL = "http://" + parsedURL
			}
			l = l.Proxy(parsedURL)
			e.logger.Info("rod using proxy", zap.String("proxy", parsedURL))
		} else {
			e.logger.Info("rod direct connection (no proxy)")
		}
	}

	e.logger.Info("rod launch config", zap.Bool("headless", headless))

	u := l.MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	return browser, nil
}

func detectCaptcha(page *rod.Page) bool {
	captchaSelectors := []string{
		"#waf_nc_block",
		"#WAF_NC_WRAPPER",
		".waf-nc-wrapper",
		"#aliyunCaptcha-sliding-slider",
		`[class*="nc-wrapper"]`,
		`[class*="captcha"]`,
	}

	// Immediate check
	if hasAnyVisible(page, captchaSelectors) {
		return true
	}

	// Wait 5s then check again (same as Python: page.locator().wait_for(state="visible", timeout=5000))
	time.Sleep(5 * time.Second)
	return hasAnyVisible(page, captchaSelectors)
}

func hasAnyVisible(page *rod.Page, selectors []string) bool {
	for _, sel := range selectors {
		el, err := page.Timeout(3 * time.Second).Element(sel)
		if err != nil {
			continue
		}
		visible, err := el.Visible()
		if err == nil && visible {
			return true
		}
	}
	return false
}
