package main

import (
	"os"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/middlewares"
	"ownstak-proxy/src/server"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	godotenv.Load(".env", ".env.local")

	// Log the app info and start the server
	provider := os.Getenv(constants.EnvProvider)
	logger.Info("%s, Version: %s, Mode: %s, Provider: %s", constants.AppName, constants.Version, constants.Mode, provider)
	server.NewServer().
		Use(middlewares.NewHealthcheckMiddleware()).
		Use(middlewares.NewRequestIdMiddleware()).
		Use(middlewares.NewServerInfoMiddleware()).
		Use(middlewares.NewServerProfilerMiddleware()).
		Use(middlewares.NewImageOptimizerMiddleware()).
		Use(middlewares.NewFollowRedirectMiddleware()).
		Use(middlewares.NewAWSLambdaMiddleware()).
		Start()
}
