package main

import (
	"context"
	"log"
	"time"

	pb "solgrpc/proto"

	"google.golang.org/grpc"
)

func main() {

	conn, err := grpc.NewClient("localhost:8080", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}

	client := pb.NewGeyserClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	version, err := client.GetVersion(ctx, &pb.GetVersionRequest{})
	if err != nil {
		log.Fatal(err)
	}

	log.Println(version.Version)
}
