# OwnStack Proxy
The OwnStack Proxy is a simple proxy server that works as API Gateway replacement for OwnStack with more features and higher limits. 
It accepts requests on HTTP/HTTPS port and proxies them to AWS Lambda by invoking the Lambda function directly with API Gateway v2 compatible payload.

## Supported features
- [x] AWS Lambda
    - [x] Invocation in BUFFERED mode
    - [ ] Invocation in STREAMING mode
    - [x] Basic error handling for Lambda functions
- [x] Following redirects to another hosts (S3, etc...)
- [ ] Response streaming
- [ ] Caching
- [ ] Basic logging
- [ ] Basic metrics

## Requirements
- GoLang 1.22+

## Installation
1. Clone the repository
```bash
git clone git@github.com:ownstack-org/ownstack-proxy.git
```

2. Install Go from package repository
```bash
# macOS
brew install go
```

```bash
# Ubuntu/Debian based distros
sudo apt install golang
```

Or install the latest version from source on [GoLang website](https://go.dev/dl/)


3. Install dependencies
```bash
./scripts/install.sh
```

## Development
Just run the following command to build the app and start the development server. 
It will automatically rebuild the app when you make changes to the code.
```bash
./scripts/dev.sh
```
If no certs are provided, it will generate self-signed cert/key/CA pairs for the development server.
After proxy server starts, you can access the proxy server at [https://site-123.aws-account.localhost:3000](https://site-123.aws-account.localhost:3000)

## Build
To build the app for the target platforms specified in the `.env` file under `PLATFORMS` variable, run the following command:
```bash
./scripts/build.sh
```

## Start
To start the built binary for current platform, you can run the following command:
```bash
./scripts/start.sh
```

Or just run it directly. It's standalone executable without any dependencies.

## Stable release
To release a new version, you need to create a new release in the GitHub UI.
After the release is created, the GitHub Actions will build the app for all the target platforms, release the docker images and attach the binaries to the release.

Steps to release a new version:
1. Create a new release in the [Releases](https://github.com/ownstack-org/ownstack-proxy/releases/new) page or use existing release draft.
2. Create a new tag with the corresponding version. The current release candidate version can be found in the `.env` file under `VERSION` variable but the release pipeline will use the version from the tag.
3. The tag name should be in the format of `v{version}`. For example, `v1.0.1`. Then click on `Create new tag` button.
4. Set the release title to same name as the tag.
5. Add or update release notes if needed.
6. Check the `Set as the latest release ` checkbox.
7. Click on `Publish release` button.
8. The release pipeline will start. You can see the progress in the [Actions](https://github.com/ownstack-org/ownstack-proxy/actions) page.
9. Once the release pipeline is finished, you can see the release binaries in the [Releases](https://github.com/ownstack-org/ownstack-proxy/releases) page.
10. Done

# Preview/Next release
Every commit to the opened PR will trigger a preview/next release build. 
The release version is in the format of `{version}-next-{timestamp}-{commit}`. For example, `1.0.0-next-1744196863-97afab`. 
You can find the actual release next version in the CI logs.