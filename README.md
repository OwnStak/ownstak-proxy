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
