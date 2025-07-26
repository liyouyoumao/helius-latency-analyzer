package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"solgrpc/proto"
	"syscall"

	"google.golang.org/grpc"
)

func main() {
	// 创建 gRPC 服务器
	s := grpc.NewServer()
	proto.RegisterGeyserServer(s, GeyserServer{})

	// 启动监听
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}

	// 使用 goroutine 启动 gRPC 服务
	go func() {
		log.Println("gRPC 服务启动在 :8080")
		if err := s.Serve(listener); err != nil {
			log.Fatalf("服务启动失败：%s", err)
		}
	}()

	// 设置优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // 等待中断信号

	log.Println("正在优雅关闭 gRPC 服务...")
	s.GracefulStop()
	log.Println("服务已关闭")
}
