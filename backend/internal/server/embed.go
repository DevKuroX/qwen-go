package server

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

//go:embed all:dashboard
var dashboardFiles embed.FS

func GetDashboardFS() (fs.FS, error) {
	sub, err := fs.Sub(dashboardFiles, "dashboard")
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func DashboardHandler() http.Handler {
	sub, err := GetDashboardFS()
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}

// DashboardExists checks if the dashboard was built and embedded
func DashboardExists() bool {
	_, err := fs.Stat(dashboardFiles, "dashboard/index.html")
	return err == nil
}

// DevDashboardHandler serves from frontend/out/ for development (no embed rebuild needed)
func DevDashboardHandler() http.Handler {
	cwd, _ := os.Getwd()
	candidates := []string{
		path.Join(cwd, "../../frontend/out"),
		path.Join(cwd, "../frontend/out"),
		path.Join(cwd, "frontend/out"),
	}
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			return http.FileServer(http.Dir(dir))
		}
	}
	return nil
}

// cleanPath removes Next.js Turbopack metadata files from directory listings
func cleanPath(p string) string {
	if strings.Contains(p, "__next.") {
		return ""
	}
	return p
}
