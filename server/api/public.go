package api

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type installerItem struct {
	Filename    string `json:"filename"`
	Platform    string `json:"platform"`
	Size        int64  `json:"size"`
	ModifiedAt  string `json:"modified_at"`
	DownloadURL string `json:"download_url"`
	Description string `json:"description"`
}

func (s *Server) listPublicInstallers(c *fiber.Ctx) error {
	baseDir := strings.TrimSpace(s.cfg.Server.PublicDownloadDir)
	if baseDir == "" {
		baseDir = "./downloads"
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil && !os.IsNotExist(err) {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]installerItem, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		name := entry.Name()
		items = append(items, installerItem{
			Filename:    name,
			Platform:    installerPlatform(name),
			Size:        info.Size(),
			ModifiedAt:  info.ModTime().UTC().Format(time.RFC3339),
			DownloadURL: c.BaseURL() + "/api/v1/public/installers/" + urlEscapePath(name),
			Description: installerDescription(name),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Platform == items[j].Platform {
			return items[i].Filename < items[j].Filename
		}
		return items[i].Platform < items[j].Platform
	})
	return c.JSON(fiber.Map{"installers": items, "directory": baseDir})
}

func (s *Server) listFlutterInstallers(c *fiber.Ctx) error {
	baseDir := "./flutter-downloads"
	entries, err := os.ReadDir(baseDir)
	if err != nil && !os.IsNotExist(err) {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]installerItem, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		name := entry.Name()
		items = append(items, installerItem{
			Filename:    name,
			Platform:    flutterInstallerPlatform(name),
			Size:        info.Size(),
			ModifiedAt:  info.ModTime().UTC().Format(time.RFC3339),
			DownloadURL: c.BaseURL() + "/api/v1/public/flutter-installers/" + urlEscapePath(name),
			Description: flutterInstallerDescription(name),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Platform == items[j].Platform {
			return items[i].Filename < items[j].Filename
		}
		return items[i].Platform < items[j].Platform
	})
	return c.JSON(fiber.Map{"installers": items})
}

func (s *Server) downloadFlutterInstaller(c *fiber.Ctx) error {
	baseDir := "./flutter-downloads"
	filename := strings.TrimSpace(c.Params("filename"))
	if filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "filename is required"})
	}
	safeName := filepath.Base(filename)
	if safeName != filename {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid filename"})
	}
	filePath := filepath.Join(baseDir, safeName)
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "installer not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	c.Attachment(safeName)
	return c.SendFile(filePath)
}

func flutterInstallerPlatform(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "windows") && strings.Contains(lower, "x64"):
		return "windows"
	case strings.Contains(lower, "macos") && strings.Contains(lower, "arm64"):
		return "macos"
	case strings.Contains(lower, "macos") && strings.Contains(lower, "x64"):
		return "macos"
	case strings.Contains(lower, "linux") && strings.Contains(lower, "x64"):
		return "linux"
	case strings.HasSuffix(lower, ".exe"):
		return "windows"
	case strings.HasSuffix(lower, ".dmg"):
		return "macos"
	case strings.HasSuffix(lower, ".deb"):
		return "linux"
	default:
		return "other"
	}
}

func flutterInstallerDescription(name string) string {
	lower := strings.ToLower(name)
	platform := flutterInstallerPlatform(name)
	switch platform {
	case "windows":
		return "Windows x64"
	case "macos":
		if strings.Contains(lower, "arm64") {
			return "macOS Apple Silicon (M1/M2/M3)"
		}
		return "macOS Intel"
	case "linux":
		return "Linux x64"
	default:
		return "Installer"
	}
}

func (s *Server) downloadInstaller(c *fiber.Ctx) error {
	baseDir := strings.TrimSpace(s.cfg.Server.PublicDownloadDir)
	if baseDir == "" {
		baseDir = "./downloads"
	}
	filename := strings.TrimSpace(c.Params("filename"))
	if filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "filename is required"})
	}
	safeName := filepath.Base(filename)
	if safeName != filename {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid filename"})
	}
	filePath := filepath.Join(baseDir, safeName)
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "installer not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	c.Attachment(safeName)
	return c.SendFile(filePath)
}

func installerPlatform(name string) string {
	lower := strings.ToLower(name)
	switch {
	// Prefer an explicit platform token in the filename over the extension, so
	// e.g. a Windows portable .zip isn't mislabelled as macOS.
	case strings.Contains(lower, "windows") || strings.HasSuffix(lower, ".exe"):
		return "windows"
	case strings.Contains(lower, "linux") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".deb"):
		return "linux"
	case strings.HasSuffix(lower, ".pkg"):
		return "macos-agent"
	case strings.HasSuffix(lower, ".dmg") || strings.Contains(lower, "macos") || strings.Contains(lower, ".app"):
		return "macos-desktop"
	case strings.HasSuffix(lower, ".zip"):
		return "macos"
	default:
		return "other"
	}
}

func installerDescription(name string) string {
	lower := strings.ToLower(name)
	platform := installerPlatform(name)
	switch platform {
	case "windows":
		return "Windows installer"
	case "macos":
		return "macOS archive"
	case "macos-desktop":
		if strings.HasSuffix(lower, ".dmg") {
			return "macOS desktop app (recommended)"
		}
		return "macOS desktop app (zip)"
	case "macos-agent":
		return "macOS agent package"
	case "linux":
		return "Debian package"
	default:
		return "Installer"
	}
}

func urlEscapePath(name string) string {
	return url.PathEscape(name)
}
