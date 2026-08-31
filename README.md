# App Creator

A Wails-based desktop GUI for converting `.deb` and `.rpm` package files into standalone AppImages.

## Overview

App Creator provides a simple interface to:

1. Select a `.deb` or `.rpm` package file
2. Optionally upload a custom application icon (PNG, SVG, XPM, ICO)
3. Optionally override the detected application name
4. Convert the package into an AppImage using the standard AppDir structure and `appimagetool`

## How it works

- The package is extracted (`ar` + `tar` for .deb, `rpm2cpio` + `cpio` for .rpm)
- Metadata is auto-discovered (`.desktop` files, AppStream `.metainfo.xml`, icon paths)
- An AppDir is built with the proper `.desktop` file, `AppRun` script, and icon
- `appimagetool` bundles the AppDir into an executable AppImage

## Prerequisites

- `appimagetool` from [AppImageKit](https://github.com/AppImage/AppImageKit)
- `ar`, `tar`, and `rpm2cpio` (standard on Linux)
- Wails v2 for development builds

## Build

```bash
./scripts/wails-build.sh
```

## Development

```bash
./scripts/wails-dev.sh
```

## License

MIT