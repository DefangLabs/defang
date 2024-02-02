#!/bin/bash

# Define the GitHub API URL for the latest release
RELEASE_API_URL="https://api.github.com/repos/defang-io/defang/releases/latest"

# Use curl to fetch the latest release data
echo "Fetching the latest release information..."
RELEASE_JSON=$(curl -s $RELEASE_API_URL)

# Check for curl failure
if [ $? -ne 0 ]; then
    echo "Error fetching release information. Please check your connection or if the URL is correct."
    exit 1
fi

# Determine system architecture and operating system
ARCH=$(uname -m)
OS=$(uname -s)

# Adjust the architecture string to match the naming convention in the download URLs
case $ARCH in
    x86_64) ARCH_SUFFIX="amd64" ;;
    arm64) ARCH_SUFFIX="arm64" ;;
    aarch64) ARCH_SUFFIX="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Initialize the download URL variable
DOWNLOAD_URL=""

# Based on the OS, filter the download URL
if [ "$OS" = "Darwin" ]; then
    DOWNLOAD_URL=$(echo "$RELEASE_JSON" | grep -o "https://github.com/defang-io/defang/releases/download/v[0-9.]*/defang_[0-9.]*_macOS.zip" | head -n 1)
elif [ "$OS" = "Linux" ]; then
    DOWNLOAD_URL=$(echo "$RELEASE_JSON" | grep -o "https://github.com/defang-io/defang/releases/download/v[0-9.]*/defang_[0-9.]*_linux_${ARCH_SUFFIX}.tar.gz" | head -n 1)
fi

# Abort if the download URL is not found
if [ -z "$DOWNLOAD_URL" ]; then
    echo "Could not find a download URL for your operating system ($OS) and architecture ($ARCH_SUFFIX)."
    exit 1
fi

echo "Downloading $DOWNLOAD_URL..."

# Define the output file name based on OS and ARCH_SUFFIX
FILENAME="defang_latest"
if [ "$OS" = "Darwin" ]; then
    FILENAME="$FILENAME.zip"
elif [ "$OS" = "Linux" ]; then
    FILENAME="$FILENAME.tar.gz"
fi

# Download the file
if ! curl -L "$DOWNLOAD_URL" -o "$FILENAME"; then
    echo "Download failed. Please check your internet connection and try again."
    exit 1
fi

# Create a temporary directory for extraction
EXTRACT_DIR=$(mktemp -d)

# Extract the downloaded file to the temporary directory
echo "Extracting the downloaded file to $EXTRACT_DIR..."
if [ "$OS" = "Darwin" ]; then
    if ! unzip "$FILENAME" -d "$EXTRACT_DIR"; then
        echo "Failed to extract the downloaded file. The file might be corrupted."
        exit 1
    fi
elif [ "$OS" = "Linux" ]; then
    if ! tar -xzf "$FILENAME" -C "$EXTRACT_DIR"; then
        echo "Failed to extract the downloaded file. The file might be corrupted."
        exit 1
    fi
fi

# Determine the installation directory
INSTALL_DIR="$HOME/.local/bin"

# Check if the installation directory exists and is writable
if [ ! -d "$INSTALL_DIR" ]; then
    echo "The installation directory ($INSTALL_DIR) does not exist. Creating it now..."
    if ! mkdir -p "$INSTALL_DIR"; then
        echo "Failed to create the installation directory. Please check your permissions and try again."
        exit 1
    fi
elif [ ! -w "$INSTALL_DIR" ]; then
    echo "The installation directory ($INSTALL_DIR) is not writable. Please check your permissions and try again."
    exit 1
fi

# Assuming the binary or application name is predictable and consistent
BINARY_NAME='defang' # Adjust this based on actual content

# Move the binary or application to the installation directory from the temporary directory
echo "Moving the binary/application to $INSTALL_DIR"
if ! mv "$EXTRACT_DIR/$BINARY_NAME" "$INSTALL_DIR"; then
    echo "Failed to move the binary/application. Please check your permissions and try again."
    exit 1
fi

# Make the binary executable
if ! chmod +x "$INSTALL_DIR/$BINARY_NAME"; then
    echo "Failed to make the binary/application executable. Please check your permissions and try again."
    exit 1
fi

# Cleanup: Remove the temporary directory
echo "Cleaning up..."
rm -rf "$EXTRACT_DIR"

# Add the installation directory to PATH if not already present
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo "Adding $INSTALL_DIR to your PATH for this session."
    export PATH="$PATH:$INSTALL_DIR"

    # Detect the shell and choose the appropriate profile file
    case $SHELL in
    */bash)
        PROFILE_FILE="$HOME/.bashrc"
        ;;
    */zsh)
        PROFILE_FILE="$HOME/.zshrc"
        ;;
    */ksh)
        PROFILE_FILE="$HOME/.kshrc"
        ;;
    # Add more cases here for other shells
    *)
        echo "Unsupported shell. Please add the following line to your shell's profile file:"
        echo "export PATH=\"\$PATH:$INSTALL_DIR\""
        PROFILE_FILE=""
        ;;
    esac

    # Append the line to the profile file, if one was found
    if [[ -n "$PROFILE_FILE" ]]; then
        echo "export PATH=\"\$PATH:$INSTALL_DIR\"" >> "$PROFILE_FILE"
        echo "Added $INSTALL_DIR to your PATH in $PROFILE_FILE. The change will take effect in new shell sessions."
    fi
fi

echo "Installation completed. You can now use defang by typing '$BINARY_NAME' in the terminal."
