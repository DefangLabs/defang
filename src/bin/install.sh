#!/bin/bash

################################################################################################
#                                                                                              #
# This script installs the latest release of defang from GitHub. It is designed                #
# to be run like this:                                                                         #
#                                                                                              #
# . <(curl -Ls https://s.defang.io/install.sh)      #
#                                                                                              #
# This allows us to do some interactive stuff where we can prompt the user for input.          #
#                                                                                              #
################################################################################################

echo "
       __     ____
  ____/ /__  / __/___ _____  ____ _
 / __  / _ \/ /_/ __ \`/ __ \/ __ \`/
/ /_/ /  __/ __/ /_/ / / / / /_/ /
\__,_/\___/_/  \__,_/_/ /_/\__, /
                          /____/
"

# Check for -y flag or CI var and set the REPLY variable to "y" if it is present
if [[ "$1" == "-y" ]] || [[ "$CI" == "1" ]] || [[ "$CI" == "true" ]]; then
    REPLY="y"
    CI="1"
fi

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
if ! curl -s -L "$DOWNLOAD_URL" -o "$FILENAME"; then
    echo "Download failed. Please check your internet connection and try again."
    exit 1
fi

# Create a temporary directory for extraction
EXTRACT_DIR=$(mktemp -d)

# Extract the downloaded file to the temporary directory
echo "Extracting the downloaded file to $EXTRACT_DIR..."
if [ "$OS" = "Darwin" ]; then
    if ! unzip -q "$FILENAME" -d "$EXTRACT_DIR"; then
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
if [ -z "$INSTALL_DIR" ]
then
    INSTALL_DIR="$HOME/.local/bin"
fi

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
echo "Moving defang to $INSTALL_DIR"
if ! mv "$EXTRACT_DIR/$BINARY_NAME" "$INSTALL_DIR"; then
    echo "Failed to move defang. Please check your permissions and try again."
    exit 1
fi

# Make the binary executable
if ! chmod +x "$INSTALL_DIR/$BINARY_NAME"; then
    echo "Failed to make defang executable. Please check your permissions and try again."
    exit 1
fi

# Cleanup: Remove the temporary directory
echo "Cleaning up..."
rm -r "$EXTRACT_DIR"

prompt_and_append_to_profile() {
    local prompt=$1
    local profile_file=$2
    local line="export PATH=\"\$PATH:$INSTALL_DIR\""
    echo "We'd like this line to your $profile_file:"
    echo
    echo "  $line"
    echo
    if [[ "$CI" != "1" ]]; then
        # Prompt the user for confirmation
        echo -n "$prompt $profile_file? (y/n) "
        read REPLY
        echo    # move to a new line
    fi
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        # Append the line to the profile file
        echo >> "$profile_file"
        echo "$line # Added by Defang install.sh" >> "$profile_file"
    else
        # Print the command for the user to run manually
        echo "To add $INSTALL_DIR to your PATH, run the following command:"
        echo
        echo "  echo '$line' >> \"$profile_file\""
        echo
    fi
}

# Get the name of the current shell
CURRENT_SHELL=$(basename "$SHELL")

# Add the installation directory to PATH if not already present
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo "Adding $INSTALL_DIR to your PATH for this session."
    export PATH="$PATH:$INSTALL_DIR"

    # Define the possible shell profile files
    PROFILE_FILES=(".bashrc" ".zshrc" ".kshrc")

    # Loop over the possible profile files
    FOUND_PROFILE_FILE=false
    for profile_file in "${PROFILE_FILES[@]}"; do
        # If the profile file exists in the user's home directory, add a line to it
        if [[ -f "$HOME/$profile_file" ]]; then
            FOUND_PROFILE_FILE=true
            prompt_and_append_to_profile "Can we append the necessary line to" "$HOME/$profile_file"
        fi
    done

    # If no profile file was found
    if [[ $FOUND_PROFILE_FILE == false ]]; then
        # Prompt the user to create a new profile file
        prompt_and_append_to_profile "No existing profile file found. Can we create" "$HOME/.${CURRENT_SHELL}rc"
    fi
fi

# TODO: Install shell completion script


# Loop over the possible profile files
for profile_file in "${PROFILE_FILES[@]}"; do
    # If the profile file exists in the user's home directory, add a line to it
    if [[ -f "$HOME/$profile_file" ]]; then
        prompt_and_append_to_profile "Can we append the necessary line to" "$HOME/$profile_file"
    fi
done

# Cleanup: Remove the originally downloaded file
rm "$FILENAME"

echo "Installation completed. You can now use defang by typing '$BINARY_NAME' in the terminal."
