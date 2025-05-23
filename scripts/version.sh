#!/bin/bash

# Fail on error
set -euo pipefail

# Path to the .env file
ENV_FILE=".env"

# Display help
display_help() {
    echo "Usage: $0 {bump|set|tag|get}"
    echo "  bump    Increment the patch version"
    echo "  set     Set the version"
    echo "  tag     Append the version tag to the version"
    echo "  get     Get the current version"
    echo ""
    echo "Example:"
    echo "  $0 bump # 1.0.0 -> 1.0.1"
    echo "  $0 set 1.0.0 # 1.0.1 -> 1.0.0"
    echo "  $0 tag next # 1.0.0 -> 1.0.0-next"
    echo "  $0 get # 1.0.0"
    exit 0
}


# Increment the patch version
bump_version() {
    # Read the current version from the .env file
    current_version=$(grep -E '^VERSION=' "$ENV_FILE" | cut -d '=' -f2)

    # Check if the version is in the correct format
    if [[ $current_version =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
        major=${BASH_REMATCH[1]}
        minor=${BASH_REMATCH[2]}
        patch=${BASH_REMATCH[3]}

        # Increment the patch version
        new_patch=$((patch + 1))
        new_version="$major.$minor.$new_patch"

        # Update the .env file with the new version
        sed -i "s/^VERSION=.*/VERSION=$new_version/" "$ENV_FILE"

        echo "✅ Bumped version from $current_version to $new_version"
    else
        echo "❌ Current version format is invalid: $current_version"
        exit 1
    fi
}

# Set the version
set_version() {
    new_version=$1
    sed -i "s/^VERSION=.*/VERSION=$new_version/" "$ENV_FILE"
    echo "✅ Set version to $new_version"
}

# Append the version tag to the version
tag_version() {
    # Read the current version from the .env file
    current_version=$(grep -E '^VERSION=' "$ENV_FILE" | cut -d '=' -f2)
    new_version="$current_version-$1"
    sed -i "s/^VERSION=.*/VERSION=$new_version/" "$ENV_FILE"
    echo "✅ Set version from $current_version to $new_version"
}

# Get the current version
get_version() {
    current_version=$(grep -E '^VERSION=' "$ENV_FILE" | cut -d '=' -f2)
    echo $current_version
}

# Check if no arguments are provided
if [ $# -eq 0 ]; then
    display_help
fi

# Parse the arguments
case "$1" in
    bump)
        bump_version
        ;;
    set)
        set_version $2
        ;;
    tag)
        tag_version $2
        ;;
    get)
        get_version
        ;;
    *)
        display_help
        ;;
esac