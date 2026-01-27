#!/bin/sh
# Codesfer Installation Script
# https://github.com/GNITOAHC/codesfer
#
# Usage:
#   curl -LsSf https://raw.githubusercontent.com/GNITOAHC/codesfer/main/scripts/install.sh | sh
#   wget -qO- https://raw.githubusercontent.com/GNITOAHC/codesfer/main/scripts/install.sh | sh
#
# Environment Variables:
#   CODESFER_INSTALL_DIR  - Installation directory (default: $HOME/.local/bin)
#   CODESFER_VERSION      - Version to install (default: latest)
#   CODESFER_BINARY       - Binary to install: codesfer, codeserver, all (default: codesfer)

set -e

# Configuration
GITHUB_REPO="GNITOAHC/codesfer"
DEFAULT_INSTALL_DIR="$HOME/.local/bin"
DEFAULT_BINARY="codesfer"

# Colors (disabled if not a terminal)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

# Print functions
info() {
    printf "${BLUE}%s${NC}\n" "$1"
}

success() {
    printf "${GREEN}%s${NC}\n" "$1"
}

warn() {
    printf "${YELLOW}%s${NC}\n" "$1"
}

error() {
    printf "${RED}error: %s${NC}\n" "$1" >&2
    exit 1
}

# Detect OS
detect_os() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
    linux)
        echo "linux"
        ;;
    darwin)
        echo "darwin"
        ;;
    *)
        error "Unsupported operating system: $os"
        ;;
    esac
}

# Detect architecture
detect_arch() {
    arch=$(uname -m)
    case "$arch" in
    x86_64 | amd64)
        echo "amd64"
        ;;
    aarch64 | arm64)
        echo "arm64"
        ;;
    *)
        error "Unsupported architecture: $arch"
        ;;
    esac
}

# Check if a command exists
has_command() {
    command -v "$1" >/dev/null 2>&1
}

# Download a file using curl or wget
download() {
    url="$1"
    output="$2"

    if has_command curl; then
        curl -fsSL "$url" -o "$output"
    elif has_command wget; then
        wget -q "$url" -O "$output"
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Fetch the latest release version from GitHub API
get_latest_version() {
    api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"

    if has_command curl; then
        version=$(curl -fsSL "$api_url" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    elif has_command wget; then
        version=$(wget -qO- "$api_url" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    else
        error "Neither curl nor wget found. Please install one of them."
    fi

    if [ -z "$version" ]; then
        error "Failed to fetch latest version from GitHub"
    fi

    echo "$version"
}

# Install a single binary
install_binary() {
    binary_name="$1"
    version="$2"
    os="$3"
    arch="$4"
    install_dir="$5"

    # Construct download URL
    # Format: {binary}-{version}-{os}-{arch}.tar.gz
    archive_name="${binary_name}-${version}-${os}-${arch}.tar.gz"
    download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${archive_name}"

    info "Downloading ${binary_name} ${version} for ${os}-${arch}..."

    # Create temp directory
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    # Download archive
    archive_path="${tmp_dir}/${archive_name}"
    download "$download_url" "$archive_path" || error "Failed to download ${binary_name} from ${download_url}"

    # Extract archive
    tar -xzf "$archive_path" -C "$tmp_dir" || error "Failed to extract archive"

    # Find the binary in extracted files
    if [ -f "${tmp_dir}/${binary_name}" ]; then
        binary_path="${tmp_dir}/${binary_name}"
    else
        error "Binary ${binary_name} not found in archive"
    fi

    # Create install directory if it doesn't exist
    mkdir -p "$install_dir" || error "Failed to create directory: ${install_dir}"

    # Move binary to install directory
    mv "$binary_path" "${install_dir}/${binary_name}" || error "Failed to install ${binary_name}"

    # Make binary executable
    chmod +x "${install_dir}/${binary_name}" || error "Failed to make ${binary_name} executable"

    success "Installed ${binary_name} to ${install_dir}/${binary_name}"
}

# Main installation function
main() {
    # Get configuration from environment or use defaults
    install_dir="${CODESFER_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    version="${CODESFER_VERSION:-latest}"
    binary="${CODESFER_BINARY:-$DEFAULT_BINARY}"

    # Detect platform
    os=$(detect_os)
    arch=$(detect_arch)

    info "Detected platform: ${os}-${arch}"

    # Resolve version
    if [ "$version" = "latest" ]; then
        info "Fetching latest version..."
        version=$(get_latest_version)
    fi

    info "Version: ${version}"
    info "Install directory: ${install_dir}"

    # Determine which binaries to install
    case "$binary" in
    codesfer)
        binaries="codesfer"
        ;;
    codeserver)
        binaries="codeserver"
        ;;
    all)
        binaries="codesfer codeserver"
        ;;
    *)
        error "Invalid binary option: ${binary}. Use 'codesfer', 'codeserver', or 'all'."
        ;;
    esac

    # Install each binary
    for bin in $binaries; do
        install_binary "$bin" "$version" "$os" "$arch" "$install_dir"
    done

    echo ""
    success "Installation complete!"
    echo ""

    # Check if install directory is in PATH
    case ":$PATH:" in
    *":${install_dir}:"*)
        info "${install_dir} is already in your PATH."
        ;;
    *)
        warn "To use codesfer, make sure ${install_dir} is in your PATH:"
        echo ""
        echo "  export PATH=\"${install_dir}:\$PATH\""
        echo ""
        echo "You can add this to your shell profile (~/.bashrc, ~/.zshrc, etc.)"
        ;;
    esac
}

main "$@"
