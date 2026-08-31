package guiapp
import (
	"bufio"
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound application struct.
type App struct {
	ctx context.Context
}

// New creates a new App instance.
func New() *App {
	return &App{}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// PickFile opens a native file dialog and returns the selected path.
func (a *App) PickFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select a package file",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Package files",
				Pattern:     "*.deb;*.rpm",
			},
			{
				DisplayName: "Debian packages",
				Pattern:     "*.deb",
			},
			{
				DisplayName: "RPM packages",
				Pattern:     "*.rpm",
			},
		},
	})
}

// PickIcon opens a native file dialog for image selection.
func (a *App) PickIcon() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select an application icon",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Images",
				Pattern:     "*.png;*.svg;*.xpm;*.ico",
			},
		},
	})
}

// ReadFileAsBase64 reads a file and returns its base64-encoded content for
// embedding in the frontend (e.g. for preview).
func (a *App) ReadFileAsBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// DefaultAppIcon returns a base64-encoded PNG of a generic app icon. We embed
// a simple 48x48 placeholder.
func (a *App) DefaultAppIcon() string {
	return defaultIconBase64
}

// --- Conversion types --------------------------------------------------------

// ConvertRequest describes what to convert.
type ConvertRequest struct {
	PackagePath string `json:"packagePath"`
	IconPath    string `json:"iconPath"`
	AppName     string `json:"appName"`
}

// ConvertResult describes the outcome.
type ConvertResult struct {
	Success      bool   `json:"success"`
	AppImagePath string `json:"appImagePath"`
	IconBase64   string `json:"iconBase64,omitempty"`
	IconPath     string `json:"iconPath,omitempty"`
	DetectedName string `json:"detectedName,omitempty"`
	Error        string `json:"error,omitempty"`
}

// PhaseEvent is emitted to the frontend to report progress.
type PhaseEvent struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// --- Conversion --------------------------------------------------------------

// ConvertPackage converts a .deb or .rpm into an AppImage.
func (a *App) ConvertPackage(req ConvertRequest) (ConvertResult, error) {
	a.emit("convert", "Conversion", "running", "Starting conversion...")

	// 1. Validate input
	if req.PackagePath == "" {
		return fail("No package file selected")
	}
	ext := strings.ToLower(filepath.Ext(req.PackagePath))
	if ext != ".deb" && ext != ".rpm" {
		return fail("Unsupported package format. Use .deb or .rpm")
	}

	// 2. Resolve appimagetool
	appimgTool, err := resolveTool("appimagetool")
	if err != nil {
		return fail("appimagetool not found. Install it from github.com/AppImage/AppImageKit")
	}

	// 3. Create temp workspace
	tmpDir, err := os.MkdirTemp("", "appcreator-*")
	if err != nil {
		return fail(fmt.Sprintf("Failed to create temp directory: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	a.emit("convert", "Conversion", "running", "Extracting package...")

	// 4. Extract
	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return fail(fmt.Sprintf("Failed to create extract dir: %v", err))
	}
	if err := extractPackage(req.PackagePath, extractDir); err != nil {
		return fail(fmt.Sprintf("Extraction failed: %v", err))
	}

	// 5. Discover app metadata
	a.emit("convert", "Conversion", "running", "Analysing package structure...")
	meta, err := discoverMetadata(extractDir, req.PackagePath)
	if err != nil {
		return fail(fmt.Sprintf("Could not discover app metadata: %v", err))
	}
	appName := req.AppName
	if appName == "" {
		appName = meta.Name
	}

	a.emit("convert", "Conversion", "running", fmt.Sprintf("Building AppDir for %s...", appName))

	// 6. Build AppDir
	appDir := filepath.Join(tmpDir, fmt.Sprintf("%s.AppDir", appName))
	if err := buildAppDir(extractDir, appDir, meta, req.IconPath, appName); err != nil {
		return fail(fmt.Sprintf("AppDir build failed: %v", err))
	}

	// 7. Run appimagetool
	a.emit("convert", "Conversion", "running", "Running appimagetool, this may take a while...")

	outDir := filepath.Dir(req.PackagePath)
	appImagePath := filepath.Join(outDir, fmt.Sprintf("%s-%s-x86_64.AppImage", appName, meta.Version))
	cmd := exec.Command(appimgTool, appDir, appImagePath)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fail(fmt.Sprintf("appimagetool failed: %v\nOutput: %s", err, string(output)))
	}

	// 8. Make executable
	if err := os.Chmod(appImagePath, 0755); err != nil {
		return fail(fmt.Sprintf("Could not chmod AppImage: %v", err))
	}

	// 9. Load the icon for preview
	iconB64 := meta.IconBase64
	if req.IconPath != "" {
		if data, err := os.ReadFile(req.IconPath); err == nil {
			iconB64 = base64.StdEncoding.EncodeToString(data)
		}
	}

	a.emit("convert", "Conversion", "complete", fmt.Sprintf("AppImage created at %s", appImagePath))

	return ConvertResult{
		Success:      true,
		AppImagePath: appImagePath,
		IconBase64:   iconB64,
		IconPath:     meta.IconPath,
		DetectedName: meta.Name,
	}, nil
}

// --- Internal helpers --------------------------------------------------------

func (a *App) emit(id, title, state, msg string) {
	runtime.EventsEmit(a.ctx, "creator:phase", PhaseEvent{
		ID:      id,
		Title:   title,
		State:   state,
		Message: msg,
	})
}

func fail(msg string) (ConvertResult, error) {
	return ConvertResult{Success: false, Error: msg}, errors.New(msg)
}

// resolveTool looks for a binary in PATH or common locations.
func resolveTool(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	// Common AppImage install paths
	paths := []string{
		"/usr/local/bin/" + name,
		"/usr/bin/" + name,
		filepath.Join(os.Getenv("HOME"), ".local/bin", name),
		filepath.Join(os.Getenv("HOME"), "bin", name),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Maybe it's an AppImage in PATH
	if path, err := exec.LookPath(name + ".AppImage"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("%s not found", name)
}

// extractPackage unpacks a .deb or .rpm into dest.
func extractPackage(src, dest string) error {
	ext := strings.ToLower(filepath.Ext(src))
	switch ext {
	case ".deb":
		return extractDeb(src, dest)
	case ".rpm":
		return extractRpm(src, dest)
	default:
		return fmt.Errorf("unsupported format: %s", ext)
	}
}

func extractDeb(debPath, dest string) error {
	// Debian archive: ar x picks data.tar.{xz,gz,bz2,zst}
	arCmd := exec.Command("ar", "x", debPath)
	arCmd.Dir = dest
	if out, err := arCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ar x failed: %v\n%s", err, string(out))
	}

	// Find and extract the data tarball
	entries, err := os.ReadDir(dest)
	if err != nil {
		return err
	}

	var dataTar string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "data.tar") {
			dataTar = filepath.Join(dest, e.Name())
			break
		}
	}
	if dataTar == "" {
		return fmt.Errorf("no data.tar.* found in deb archive")
	}

	// Determine decompression flag
	var decompressFlag string
	switch {
	case strings.HasSuffix(dataTar, ".xz"):
		decompressFlag = "-J"
	case strings.HasSuffix(dataTar, ".gz"):
		decompressFlag = "-z"
	case strings.HasSuffix(dataTar, ".bz2"):
		decompressFlag = "-j"
	case strings.HasSuffix(dataTar, ".zst"):
		decompressFlag = "--zstd"
	default:
		decompressFlag = ""
	}

	args := []string{"xf"}
	if decompressFlag != "" {
		args = append(args, decompressFlag)
	}
	args = append(args, dataTar)

	tarCmd := exec.Command("tar", args...)
	tarCmd.Dir = dest
	if out, err := tarCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar extraction failed: %v\n%s", err, string(out))
	}

	return nil
}

func extractRpm(rpmPath, dest string) error {
	// rpm2cpio extracts to stdout, then cpio extracts in dest
	// Try rpm2cpio first (part of rpm), fallback to rpm2cpio.sh
	cmd := exec.Command("rpm2cpio", rpmPath)
	cpio := exec.Command("cpio", "-idmv")
	cpio.Dir = dest

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cpio.Stdin = pipe

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("rpm2cpio failed: %v", err)
	}
	if err := cpio.Start(); err != nil {
		return fmt.Errorf("cpio failed: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("rpm2cpio failed: %v", err)
	}
	if err := cpio.Wait(); err != nil {
		return fmt.Errorf("cpio extraction failed: %v", err)
	}

	return nil
}

// appMetadata describes an application discovered in a package.
type appMetadata struct {
	Name         string
	Version      string
	Executable   string
	DesktopFile  string
	AppStream    string
	IconName     string
	IconPath     string
	IconBase64   string
	Categories   string
	Comment      string
}

// discoverMetadata scans an extracted package for app metadata.
func discoverMetadata(extractDir, pkgPath string) (*appMetadata, error) {
	meta := &appMetadata{
		Name:    guessAppName(pkgPath),
		Version: "1.0",
	}

	// Look for .desktop files
	desktopFiles := findFiles(extractDir, ".desktop")
	if len(desktopFiles) > 0 {
		meta.DesktopFile = desktopFiles[0]
		parseDesktopFile(meta, desktopFiles[0])
	}

	// Look for appstream metadata
	metainfoFiles := findFiles(extractDir, ".metainfo.xml")
	appdataFiles := findFiles(extractDir, ".appdata.xml")
	allMeta := append(metainfoFiles, appdataFiles...)
	if len(allMeta) > 0 {
		parseAppstream(meta, allMeta[0])
	}

	// Look for icons
	if meta.IconName != "" {
		icon := findIcon(extractDir, meta.IconName)
		if icon != "" {
			meta.IconPath = icon
			if data, err := os.ReadFile(icon); err == nil {
				meta.IconBase64 = base64.StdEncoding.EncodeToString(data)
			}
		}
	}

	// Fallback: pick the first PNG icon
	if meta.IconPath == "" {
		icons := findFiles(extractDir, ".png")
		// Prefer larger icons in hicolor or in /usr/share/icons
		sortIconCandidates(icons, extractDir)
		if len(icons) > 0 {
			meta.IconPath = icons[0]
			if data, err := os.ReadFile(icons[0]); err == nil {
				meta.IconBase64 = base64.StdEncoding.EncodeToString(data)
			}
		}
	}

	// Look for executables
	if meta.Executable == "" {
		binDir := filepath.Join(extractDir, "usr", "bin")
		if entries, err := os.ReadDir(binDir); err == nil && len(entries) > 0 {
			meta.Executable = filepath.Join("usr", "bin", entries[0].Name())
		}
	}

	return meta, nil
}

func guessAppName(pkgPath string) string {
	base := filepath.Base(pkgPath)
	base = strings.TrimSuffix(base, ".deb")
	base = strings.TrimSuffix(base, ".rpm")
	// Strip version suffix (e.g. myapp_1.2.3 → myapp)
	if idx := strings.LastIndex(base, "_"); idx > 0 {
		base = base[:idx]
	}
	// Strip arch suffix
	for _, suf := range []string{".x86_64", ".amd64", ".i386", ".noarch", "-x86_64", "-amd64"} {
		base = strings.TrimSuffix(base, suf)
	}
	if base == "" {
		return "App"
	}
	return titleCase(base)
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
func parseDesktopFile(meta *appMetadata, path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "Name=") && meta.Name == "":
			meta.Name = strings.TrimPrefix(line, "Name=")
		case strings.HasPrefix(line, "Exec="):
			exe := strings.TrimPrefix(line, "Exec=")
			// Strip %f, %F, %u, %U etc
			parts := strings.Fields(exe)
			if len(parts) > 0 {
				meta.Executable = parts[0]
			}
		case strings.HasPrefix(line, "Icon="):
			meta.IconName = strings.TrimPrefix(line, "Icon=")
		case strings.HasPrefix(line, "Categories="):
			meta.Categories = strings.TrimPrefix(line, "Categories=")
		case strings.HasPrefix(line, "Comment="):
			meta.Comment = strings.TrimPrefix(line, "Comment=")
		}
	}
}
func parseAppstream(meta *appMetadata, path string) {
	// Rudimentary XML parsing — just grab <id> and <summary>
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)

	if idIdx := strings.Index(content, "<id>"); idIdx >= 0 {
		endIdx := strings.Index(content[idIdx+4:], "</id>")
		if endIdx >= 0 {
			id := content[idIdx+4 : idIdx+4+endIdx]
			if strings.Contains(id, ".") {
				parts := strings.SplitN(id, ".", 2)
				if len(parts) > 0 {
					meta.Name = parts[len(parts)-1]
				}
			}
		}
	}

	if sumIdx := strings.Index(content, "<summary>"); sumIdx >= 0 {
		endIdx := strings.Index(content[sumIdx+9:], "</summary>")
		if endIdx >= 0 {
			meta.Comment = content[sumIdx+9 : sumIdx+9+endIdx]
		}
	}
}

func findIcon(extractDir, iconName string) string {
	// Search standard icon paths
	searchPaths := []string{
		filepath.Join("usr", "share", "icons", "hicolor", "256x256", "apps"),
		filepath.Join("usr", "share", "icons", "hicolor", "128x128", "apps"),
		filepath.Join("usr", "share", "icons", "hicolor", "64x64", "apps"),
		filepath.Join("usr", "share", "icons", "hicolor", "48x48", "apps"),
		filepath.Join("usr", "share", "icons", "hicolor", "scalable", "apps"),
		filepath.Join("usr", "share", "icons"),
		filepath.Join("usr", "share", "pixmaps"),
	}

	extensions := []string{".png", ".svg", ".xpm"}
	for _, sp := range searchPaths {
		for _, ext := range extensions {
			p := filepath.Join(extractDir, sp, iconName+ext)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

func findFiles(root, ext string) []string {
	var result []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ext) {
			result = append(result, path)
		}
		return nil
	})
	return result
}

func sortIconCandidates(paths []string, extractDir string) {
	// Simple bubble: prefer paths with "hicolor/256" or "hicolor/128"
	// and prefer larger sizes. We just return as-is for now — the
	// first PNG found via findFiles is already decent.
	_ = extractDir // placeholder for future smarter sorting
}

//go:embed default-icon.png
var defaultIconData []byte

var defaultIconBase64 string

func init() {
	defaultIconBase64 = base64.StdEncoding.EncodeToString(defaultIconData)
}

// buildAppDir constructs an AppDir from an extracted package.
func buildAppDir(extractDir, appDir string, meta *appMetadata, customIcon, appName string) error {
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return err
	}

	// Move usr/ into AppDir
	usrSrc := filepath.Join(extractDir, "usr")
	usrDst := filepath.Join(appDir, "usr")
	if _, err := os.Stat(usrSrc); err == nil {
		if err := os.Rename(usrSrc, usrDst); err != nil {
			return fmt.Errorf("move usr failed: %v", err)
		}
	}

	// If there's an opt/, move it too
	optSrc := filepath.Join(extractDir, "opt")
	optDst := filepath.Join(appDir, "opt")
	if _, err := os.Stat(optSrc); err == nil {
		os.Rename(optSrc, optDst)
	}

	// Write .desktop file
	desktopContent := buildDesktopEntry(meta, appName)
	desktopPath := filepath.Join(appDir, fmt.Sprintf("%s.desktop", strings.ToLower(appName)))
	if err := os.WriteFile(desktopPath, []byte(desktopContent), 0644); err != nil {
		return err
	}

	// Place icon
	iconTarget := filepath.Join(appDir, fmt.Sprintf("%s.png", strings.ToLower(appName)))
	if customIcon != "" {
		copyFile(customIcon, iconTarget)
	} else if meta.IconPath != "" {
		copyFile(meta.IconPath, iconTarget)
	} else {
		// Write a minimal 1x1 transparent PNG as fallback
		os.WriteFile(iconTarget, defaultIconData, 0644)
	}

	// Write AppRun script
	appRunContent := buildAppRun(meta, appName)
	appRunPath := filepath.Join(appDir, "AppRun")
	if err := os.WriteFile(appRunPath, []byte(appRunContent), 0755); err != nil {
		return err
	}

	return nil
}

func buildDesktopEntry(meta *appMetadata, appName string) string {
	iconName := strings.ToLower(appName)
	execPath := meta.Executable
	if execPath == "" {
		execPath = appName
	}
	// If exec is in usr/bin, make it relative to AppDir
	if !strings.HasPrefix(execPath, "/") {
		execPath = filepath.Join("usr", "bin", execPath)
	}

	categories := meta.Categories
	if categories == "" {
		categories = "Utility"
	}

	comment := meta.Comment
	if comment == "" {
		comment = fmt.Sprintf("%s packaged as AppImage", appName)
	}

	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Exec=%s
Icon=%s
Categories=%s
Comment=%s
Terminal=false
`, appName, execPath, iconName, categories, comment)
}

func buildAppRun(meta *appMetadata, appName string) string {
	execPath := meta.Executable
	if execPath == "" {
		execPath = appName
	}

	// If exec is absolute, make it relative
	if strings.HasPrefix(execPath, "/") {
		execPath = "." + execPath
	} else if !strings.HasPrefix(execPath, ".") {
		execPath = filepath.Join("./usr", "bin", execPath)
	}

	return fmt.Sprintf(`#!/bin/bash
# AppRun for %s
SELF="$(readlink -f "$0")"
HERE="$(dirname "$SELF")"
export PATH="$HERE/usr/bin:$PATH"
export LD_LIBRARY_PATH="$HERE/usr/lib:$HERE/usr/lib/x86_64-linux-gnu:$LD_LIBRARY_PATH"
export XDG_DATA_DIRS="$HERE/usr/share:$XDG_DATA_DIRS"
exec "$HERE/%s" "$@"
`, appName, execPath)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}