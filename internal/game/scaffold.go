package game

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

const (
	scaffoldDirMode  = 0o755
	scaffoldFileMode = 0o644
	scaffoldExeMode  = 0o755
)

var (
	//go:embed testdata/Info.plist
	scaffoldInfoPlist string
	//go:embed testdata/qt-Info.plist
	scaffoldQtInfoPlist string
	//go:embed testdata/SteamConfig.txt
	scaffoldSteamConfig string
	//go:embed testdata/AnalyticsConfig.txt
	scaffoldAnalyticsConfig string
)

func Scaffold(ctx context.Context, dir string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("game: %w", err)
	}
	app := filepath.Join(dir, bundleName)
	fw := filepath.Join(app, "Contents", "Frameworks")
	resources := filepath.Join(app, "Contents", "Resources")
	qt := filepath.Join(fw, "QtCore.framework", "Versions", "5")
	dirs := []string{
		filepath.Join(app, "Contents", "MacOS"),
		fw,
		filepath.Join(app, "Contents", "PlugIns", "platforms"),
		filepath.Join(app, "Contents", "PlugIns", "imageformats"),
		filepath.Join(resources, "DataOSX"),
		filepath.Join(resources, "CommonData"),
		filepath.Join(qt, "Resources"),
	}
	files := []struct {
		path string
		body string
		mode os.FileMode
	}{
		{Executable(app), "game", scaffoldExeMode},
		{filepath.Join(app, "Contents", "Info.plist"), scaffoldInfoPlist, scaffoldFileMode},
		{filepath.Join(resources, "DataOSX", "SteamConfig.txt"), scaffoldSteamConfig, scaffoldFileMode},
		{filepath.Join(resources, "CommonData", "AnalyticsConfig.txt"), scaffoldAnalyticsConfig, scaffoldFileMode},
		{filepath.Join(qt, "QtCore"), "qtcore", scaffoldExeMode},
		{filepath.Join(qt, "Resources", "Info.plist"), scaffoldQtInfoPlist, scaffoldFileMode},
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, scaffoldDirMode); err != nil {
			return "", fmt.Errorf("game: %w", err)
		}
	}
	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.body), f.mode); err != nil {
			return "", fmt.Errorf("game: %w", err)
		}
	}
	return app, nil
}
