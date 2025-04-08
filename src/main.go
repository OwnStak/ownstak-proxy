package main

import (
	"ownstack-proxy/src/constants"
	"ownstack-proxy/src/logger"
	"ownstack-proxy/src/middlewares"
	"ownstack-proxy/src/server"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	godotenv.Load()

	// Log the app info and start the server
	logger.Info("%s, Version: %s", constants.AppName, constants.Version)
	server.NewServer().
		Use(middlewares.NewRequestIdMiddleware()).
		Use(middlewares.NewServerInfoMiddleware()).
		Use(middlewares.NewAWSLambdaMiddleware()).
		Use(middlewares.NewFollowRedirectMiddleware()).
		Start()
}
