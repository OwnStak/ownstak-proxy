#!/bin/bash
source .env
set -euo pipefail

if [ ! -d $DIST_DIR ]; then
    echo "❌ Dist directory not found. Please run scripts/build.sh first."
    exit 1
fi

# Check if AWS-CLI is installed
if ! command -v aws &> /dev/null; then
   echo "❌ AWS-CLI was not found"
   exit 1
else
    echo "✅ AWS-CLI found"
fi

# Create ~/.aws/credentials file with default profile
if [ ! -f ~/.aws/credentials ]; then
    mkdir -p ~/.aws
    echo "[default]" > ~/.aws/credentials
    echo "aws_access_key_id=$AWS_ACCESS_KEY_ID" >> ~/.aws/credentials
    echo "aws_secret_access_key=$AWS_SECRET_ACCESS_KEY" >> ~/.aws/credentials
    echo "✅ Created ~/.aws/credentials file"
else
    echo "✅ ~/.aws/credentials file found"
fi

aws --profile default ecr get-login-password --region $AWS_REGION | sudo docker login --username AWS --password-stdin $REGISTRY_URL

# Build for each platform
echo "Releasing $APP_NAME $VERSION Docker images:"
sudo docker buildx create --use

PLATFORMS_ARRAY=($(echo $PLATFORMS | tr ',' '\n'))
for PLATFORM in "${PLATFORMS_ARRAY[@]}"; do
    echo "Releasing Docker image for $PLATFORM..."
    OS=$(echo $PLATFORM | cut -d '/' -f 1)
    ARCH=$(echo $PLATFORM | cut -d '/' -f 2)
    sudo docker buildx build --build-arg OS=$OS --build-arg ARCH=$ARCH --build-arg DIST_NAME=$DIST_NAME --platform "$PLATFORM" --squash -t $REGISTRY_URL/$DIST_NAME:$VERSION-$ARCH --push .
done

# Create and push a manifest list for the multi-architecture images
sudo docker buildx imagetools create --tag $REGISTRY_URL/$DIST_NAME:$VERSION $(for PLATFORM in "${PLATFORMS_ARRAY[@]}"; do ARCH=$(echo $PLATFORM | cut -d'/' -f2); echo "$REGISTRY_URL/$DIST_NAME:$VERSION-$ARCH"; done)
echo "✅ Release complete!"
echo "Image: $REGISTRY_URL/$DIST_NAME:$VERSION"