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

## Limitations

The converter makes a best-effort attempt to discover application metadata and
structure, but not every `.deb` or `.rpm` produces a working AppImage:

- **Debian maintainer scripts** (`preinst`, `postinst`, `prerm`, `postrm`) are
  not executed. Packages that depend on setup-time configuration (service
  registration, user creation, debconf prompts) will be broken.

- **Dependencies are not bundled.** The converter copies only what is inside
  the package file. If the application depends on shared libraries, interpreters,
  or runtimes not included in the package itself (`libc`, `glib`, `dotnet`,
  `mono`, `python`, `java`, etc.), it will fail at runtime.

- **RPM trigger scripts** (`triggerpostin`, `triggerun`) are not executed. The
  package state after extraction may differ from an installed system.

- **Absolute symlinks** inside the package (e.g. `/usr/lib → /usr/lib64`) point
  into the host filesystem and will break inside the AppDir.

- **`/opt/`-based applications** are supported but the converter picks the
  first executable it finds inside the first `opt/` subdirectory. Large
  multi-binary suites (e.g. an IDE with launcher, cli, and backend daemon) may
  result in a wrong `Exec` line. Verify the generated `.desktop` file if the
  AppImage launches the wrong binary.

- **Systemd units, D-Bus services, and polkit policies** shipped in the package
  are extracted but not registered. Features relying on these will not work in
  the AppImage sandbox.

- **Metadata fallback.** If the package contains no `.desktop` file and no
  AppStream metadata, the converter guesses the app name from the filename and
  uses a generic `Utility` category. These AppImages will lack a proper
  launcher icon in desktop environments.

## License

MIT