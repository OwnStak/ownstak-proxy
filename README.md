# OwnStak Proxy
The OwnStak Proxy is a simple proxy server that works as API Gateway replacement for OwnStak with more features and higher limits. 
It accepts requests on HTTP/HTTPS port and proxies them to AWS Lambda by invoking the Lambda function directly with API Gateway v2 compatible payload.

## Features
- [x] AWS Lambda
    - [x] Invocation in BUFFERED mode
    - [ ] Invocation in STREAMING mode
    - [x] Basic error handling for Lambda functions
- [x] Following redirects to another hosts (S3, etc...)
- [x] Image Optimization
- [x] Response streaming
    - [x] Streaming assets from S3 directly to client
    - [ ] Streaming response from Lambda directly to client
- [ ] Caching
- [ ] Basic logging
- [ ] Basic metrics

## Internal endpoints
All internal endpoints are prefixed with `/__ownstak__/` to prevent collisions with user-facing routes. Following internal endpoints are available:
- `/__ownstak__/health` - Healthcheck middleware endpoint. Returns a 200 OK response when the server is up and running.
- `/__ownstak__/info` - Returns useful runtime information about the server instance, such as RSS (memory usage), version, platform, etc...
- `/__ownstak__/image` - Image Optimizer endpoint. Allows to optimize images hosted on the same domain.

## Requirements
- **GoLang 1.22+**
- **glib/glibc/libc6-compat** - *Usually it's part of the system. Just minimal Alpine Linux images don't have it.*

## Installation
### 1. Clone the repository
```bash
git clone git@github.com:ownstak-org/ownstak-proxy.git
```

### 2. Install Go from package repository
```bash
# macOS
brew install go
```

```bash
# Ubuntu/Debian based distros
sudo apt install golang
```

Or install the latest version from source on [GoLang website](https://go.dev/dl/)


### 3. Install go packages
```bash
./scripts/install.sh
```

### 4. Optional: Install libvips for Image Optimizer   
The libvips is dynamic library needed for the Image Optimizer middleware to work. 
The lib folder already contains the prebuilt binaries for the most popualr platforms with included all dependencies for webp,jpeg,png...formats. 
If the pre-built libvips binary from `./lib` doesn't work for you for some reason, you might need to install it manually to your system from the repo. 
The OwnStak Proxy is also possible to build and use even without libvips. The Image Optimizater middleware will then act as simple proxy and return image unchanged.

You can install it by running:

```bash
# macOS
brew install vips glibc
```

```bash
# Ubuntu/Debian based distros
sudo apt-get install libvips libc6
```

If `./lib` folder is empty for you, you can download binaries by running `./scripts/install-libvips.sh`.
For libvips debugging, set `VIPS_DEBUG=true` env variable.

## Development
Just run the following command to build the app and start the development server. 
It will automatically rebuild the app when you make changes to the code.
```bash
./scripts/dev.sh
```
If no certs are provided, it will generate self-signed cert/key/CA pairs for the development server.
After proxy server starts, you can access the proxy server at [http://project-prod-123.aws-primary.org.localhost.ownstak.link:3000](http://project-prod-123.aws-primary.org.localhost.ownstak.link:3000)

### Lambda invocation from localhost
The upper wildcard link allows you to invoke any (for example: `sha256(project-prod-123)`) deployment lambda in your AWS account and return response.
The `.localhost.ownstak.link` domain suffix is a special case that always points to your local proxy instance. 
The production links have just `.ownstak.link` suffix.

The links are in the following format:
```
<project>-<environment>-<optional-deployment-id>.<cloud-backend>.<organization>.localhost.ownstak.link
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

Or just run it directly. It's standalone executable without any dependencies.

## Stable release
To release a new version, you need to create a new release in the GitHub UI.
After the release is created, the GitHub Actions will build the app for all the target platforms, release the docker images and attach the binaries to the release.

Steps to release a new version:
1. Create a new release in the [Releases](https://github.com/ownstak-org/ownstak-proxy/releases/new) page or use existing release draft.
2. Create a new tag with the corresponding version. The current release candidate version can be found in the `.env` file under `VERSION` variable but the release pipeline will use the version from the tag.
3. The tag name should be in the format of `v{version}`. For example, `v1.0.1`. Then click on `Create new tag` button.
4. Set the release title to same name as the tag.
5. Add or update release notes if needed.
6. Check the `Set as the latest release ` checkbox.
7. Click on `Publish release` button.
8. The release pipeline will start. You can see the progress in the [Actions](https://github.com/ownstak-org/ownstak-proxy/actions) page.
9. Once the release pipeline is finished, you can see the release binaries in the [Releases](https://github.com/ownstak-org/ownstak-proxy/releases) page.
10. Done

# Preview/Next release
Every commit to the opened PR will trigger a preview/next release build. 
The release version is in the format of `{version}-next-{timestamp}-{commit}`. For example, `1.0.0-next-1744196863-97afab`. 
You can find the actual released next version in the CI logs.
