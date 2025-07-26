package main

import (
	"context"
	"log"
	"os"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	laserEndpoint = "https://laserstream-mainnet-sgp.helius-rpc.com"
	laserXToken   = "xxx"
)

const (
	dedicatedEndpoint = "https://autostrade-diamagnets-dkdcaqjdac-dedicated-lb.helius-rpc.com:2053"
	dedicatedXToken   = "xxx"
)

func main() {
	file, err := os.OpenFile("metrics.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logrus.Fatalf("无法打开日志文件: %v", err)
	}
	// Set log level to debug
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(file)
	eg, _ := errgroup.WithContext(context.Background())
	eg.Go(func() error {
		gc, err := NewGrpcClient("laser", laserEndpoint, laserXToken, 100)
		if err != nil {
			return err
		}

		gc.SubscribeBlock()
		return nil
	})

	eg.Go(func() error {
		gc, err := NewGrpcClient("dedicated", dedicatedEndpoint, dedicatedXToken, 100)
		if err != nil {
			return err
		}

		gc.SubscribeBlock()
		return nil
	})

	if err := eg.Wait(); err != nil {
		log.Fatal(err)
	}
}
