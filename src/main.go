package main

import (
	"os"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/middlewares"
	"ownstak-proxy/src/server"
	"ownstak-proxy/src/utils"
)

func main() {
	// Log the app info and start the server
	pid := os.Getpid()
	provider := utils.GetEnv(constants.EnvProvider)
	logger.Info("%s, Version: %s, Mode: %s, Provider: %s, PID: %d", constants.AppName, constants.Version, constants.Mode, provider, pid)
	server.NewServer().
		Use(middlewares.NewHealthcheckMiddleware()).
		Use(middlewares.NewServerInfoMiddleware()).
		Use(middlewares.NewServerProfilerMiddleware()).
		Use(middlewares.NewImageOptimizerMiddleware()).
		Use(middlewares.NewFollowRedirectMiddleware()).
		Use(middlewares.NewAWSLambdaMiddleware()).
		Start()
}
