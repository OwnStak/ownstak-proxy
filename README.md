# Lambda Proxy
The Lambda Proxy is a simple proxy server that accepts requests on HTTP/HTTPS port and proxies them to AWS Lambda
by invoking the Lambda function directly with API Gateway v2 compatible payload.

## Requirements
- GoLang 1.22+

## Installation
1. Clone the repository
```bash
git clone https://github.com/ownstack-org/ownstack-proxy.git
```

2. Install Go from repository
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
go install
```

## Development
Just run the following command to build the app and start the development server. 
It will automatically rebuild the app when you make changes to the code.
```bash
./scripts/dev.sh
```

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

Or just it directly. It's standalone executable without any dependencies.
