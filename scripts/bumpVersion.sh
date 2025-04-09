#!/bin/bash

# Path to the .env file
ENV_FILE=".env"

# Function to increment the patch version
increment_patch_version() {
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

        echo "Updated version from $current_version to $new_version"
    else
        echo "Current version format is invalid: $current_version"
        exit 1
    fi
}

# Call the function to increment the patch version
increment_patch_version