package main

import (
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
	logger.Info("%s, Version: %s, Mode: %s", constants.AppName, constants.Version, constants.Mode)
	server.NewServer().
		Use(middlewares.NewRequestIdMiddleware()).
		Use(middlewares.NewServerInfoMiddleware()).
		Use(middlewares.NewImageOptimizerMiddleware()).
		Use(middlewares.NewAWSLambdaMiddleware()).
		Use(middlewares.NewFollowRedirectMiddleware()).
		Start()
}
