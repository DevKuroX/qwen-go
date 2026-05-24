// Standalone smoke test for the gemini-web provider. Loads cookies from a
// Netscape cookies.txt, runs Init/Refresh/Chat/ImageGen end-to-end, and prints
// each step. NOT linked into the main binary — used ad-hoc.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
	"github.com/qwenpi/qwenpi-go/internal/providers/geminiweb"
)

func main() {
	cookiesPath := flag.String("cookies", "/home/ubuntu/workai/qwen-go/gemini.google.com_cookies.txt", "Netscape cookies file")
	model := flag.String("model", "gemini-3-flash", "model id (gemini-3-flash / gemini-3-pro / nano-banana)")
	inputImage := flag.String("input-image", "/home/ubuntu/workai/qwen-go/Logo futuristik DevKuroX dengan efek neon(1).png", "input image path for Step 4 (edit mode)")
	flag.Parse()

	psid, psidts, err := parseCookies(*cookiesPath)
	if err != nil {
		die("parse cookies: %v", err)
	}
	fmt.Printf("[ok] loaded cookies (PSID=%s..., PSIDTS=%s...)\n", trunc(psid, 12), trunc(psidts, 12))

	pool := core.NewAccountPool()
	registry := core.NewProviderRegistry("/tmp/qwen-go-smoke/provider_configs.json")

	acc := &models.Account{
		Email:        "gw-smoke@local",
		Provider:     "gemini-web",
		Status:       models.StatusValid,
		Token:        psid,
		RefreshToken: psidts,
	}
	pool.AddAccount(acc)

	prov := geminiweb.NewProvider(pool, registry)

	// ---- Step 1: EnsureFresh (Init + rotate) ----------------------------
	fmt.Println("\n=== Step 1: EnsureFresh ===")
	t0 := time.Now()
	if err := prov.EnsureFresh(acc); err != nil {
		die("EnsureFresh failed: %v", err)
	}
	fmt.Printf("[ok] EnsureFresh in %s; PSIDTS now %s...\n", time.Since(t0), trunc(acc.RefreshToken, 12))

	// ---- Step 2: ChatCompletion -----------------------------------------
	fmt.Println("\n=== Step 2: ChatCompletion ===")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	resp, err := prov.ChatCompletion(ctx, &models.ChatRequest{
		Model: *model,
		Messages: []models.ChatMessage{
			{Role: "user", Content: "Reply with exactly: SMOKE_OK"},
		},
	})
	if err != nil {
		die("ChatCompletion failed: %v", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		die("ChatCompletion: no choices")
	}
	text := resp.Choices[0].Message.Content
	fmt.Printf("[ok] reply (%d chars): %s\n", len(text), trunc(text, 200))

	// ---- Step 3: ImageGeneration (plain) --------------------------------
	fmt.Println("\n=== Step 3: ImageGeneration ===")
	imgCtx, imgCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer imgCancel()
	imgResp, err := prov.ImageGeneration(imgCtx, &models.ImageRequest{
		Model:  *model,
		Prompt: "A small red apple on a white background, studio lighting",
	})
	var firstImageURL string
	if err != nil {
		fmt.Printf("[fail] ImageGeneration: %v\n", err)
	} else if len(imgResp.Data) == 0 {
		fmt.Printf("[fail] ImageGeneration: empty data\n")
	} else {
		firstImageURL = imgResp.Data[0].URL
		fmt.Printf("[ok] got %d image(s); first URL: %s\n", len(imgResp.Data), trunc(firstImageURL, 100))
	}

	// ---- Step 4: Image-edit with a real photographic input -------------
	fmt.Println("\n=== Step 4: Image edit (real input) ===")
	rawImage, err := os.ReadFile(*inputImage)
	if err != nil {
		fmt.Printf("[skip] read input image: %v\n", err)
		_ = firstImageURL
		return
	}
	mime := "image/png"
	if strings.HasSuffix(strings.ToLower(*inputImage), ".jpg") || strings.HasSuffix(strings.ToLower(*inputImage), ".jpeg") {
		mime = "image/jpeg"
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(rawImage)
	fmt.Printf("[info] loaded %d bytes from %s\n", len(rawImage), *inputImage)

	editCtx, editCancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer editCancel()
	editResp, err := prov.ImageGeneration(editCtx, &models.ImageRequest{
		Model:  *model,
		Prompt: "Edit this logo image: change the dominant blue/cyan color scheme to a warm orange and red color scheme. Keep the same layout, text, and composition. Output the edited image.",
		Image:  dataURL,
	})
	if err != nil {
		fmt.Printf("[fail] Image edit: %v\n", err)
		_ = firstImageURL
		return
	}
	fmt.Printf("[ok] edit produced %d image(s); first URL: %s\n", len(editResp.Data), trunc(editResp.Data[0].URL, 100))
	_ = firstImageURL
}

func parseCookies(path string) (psid, psidts string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		name, val := fields[5], fields[6]
		switch name {
		case "__Secure-1PSID":
			psid = val
		case "__Secure-1PSIDTS":
			psidts = val
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if psid == "" || psidts == "" {
		return "", "", fmt.Errorf("missing __Secure-1PSID or __Secure-1PSIDTS")
	}
	return psid, psidts, nil
}

func downloadAsBase64(rawURL, psid, psidts string) (string, string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", "", err
	}
	req.AddCookie(&http.Cookie{Name: "__Secure-1PSID", Value: psid, Domain: ".google.com"})
	req.AddCookie(&http.Cookie{Name: "__Secure-1PSIDTS", Value: psidts, Domain: ".google.com"})
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return "", "", err
	}
	mime := resp.Header.Get("Content-Type")
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	mime = strings.TrimSpace(mime)
	if mime == "" {
		mime = "image/png"
	}
	return base64.StdEncoding.EncodeToString(body), mime, nil
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[fatal] "+format+"\n", args...)
	os.Exit(1)
}

// tinyRedPNG returns a 512x512 PNG with a red circle on white — enough
// content for gemini's edit model to recognize and transform.
func tinyRedPNG() []byte {
	const size = 512
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	red := color.RGBA{R: 220, G: 30, B: 30, A: 255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, white)
		}
	}
	cx, cy, r := size/2, size/2, size/3
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, red)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
