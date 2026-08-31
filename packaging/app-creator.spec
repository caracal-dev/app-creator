%global debug_package %{nil}
%undefine _disable_source_fetch
%global upstream_version %{?version_override}%{!?version_override:0.2.2}
%global github_owner %{?github_owner_override}%{!?github_owner_override:caracal-dev}
%global github_repo %{?github_repo_override}%{!?github_repo_override:app-creator}
%global source_tag %{?source_tag_override}%{!?source_tag_override:v%{upstream_version}}
%global source_dir_name %{github_repo}-%{upstream_version}

Name:           app-creator
Version:        %{upstream_version}
Release:        %{?release_override}%{!?release_override:1}%{?dist}
Summary:        Convert .deb and .rpm packages into AppImages
License:        MIT
Requires:  appimagetool
Requires:  binutils
Requires:  cpio
Requires:  rpm-build
BuildRequires:  golang >= 1.21
URL:            https://github.com/%{github_owner}/%{github_repo}
Source0:        %{url}/archive/refs/tags/%{source_tag}.tar.gz#/%{name}-%{version}.tar.gz
BuildRequires:  glib2-devel
BuildRequires:  gtk3-devel
BuildRequires:  pkgconf-pkg-config
BuildRequires:  webkit2gtk4.1-devel
BuildRequires:  nodejs >= 18
BuildRequires:  npm

%description
app-creator provides a Wails-based desktop GUI for converting .deb and .rpm
package files into AppImages. It extracts the package, discovers icons and
metadata, builds an AppDir structure, and runs appimagetool to produce a
standalone portable AppImage.

%prep
%autosetup -n %{source_dir_name}

%build
mkdir -p build

# Build frontend
cd frontend
npm install --ignore-scripts
npm run build
cd ..

# Build Go binary
export GOTOOLCHAIN=auto
export GOFLAGS="-buildmode=pie -trimpath -mod=vendor"
go build -tags="desktop,production,webkit2_41" -ldflags="-s -w" -o build/app-creator .

%check
export GOTOOLCHAIN=auto
export GOFLAGS="-mod=vendor"
go test ./...

%install
install -d %{buildroot}%{_bindir}
install -d %{buildroot}%{_datadir}/app-creator
install -d %{buildroot}%{_datadir}/pixmaps

install -pm0755 build/app-creator %{buildroot}%{_bindir}/app-creator
cp -a frontend/dist %{buildroot}%{_datadir}/app-creator/
cp -a assets %{buildroot}%{_datadir}/app-creator/ 2>/dev/null || true
install -Dpm0644 packaging/app-creator.desktop %{buildroot}%{_datadir}/applications/app-creator.desktop

%files
%license LICENSE
%doc README.md
%{_bindir}/app-creator
%{_datadir}/app-creator/dist/index.html
%{_datadir}/app-creator/dist/main.css
%{_datadir}/app-creator/dist/main.js
%{_datadir}/app-creator/dist/wailsjs/**
%{_datadir}/app-creator/assets/**
%{_datadir}/applications/app-creator.desktop

%changelog
* Sun Aug 31 2026 Atumia <atumia@users.noreply.github.com> - %{version}-%{release}
- Initial package: App Creator desktop GUI
