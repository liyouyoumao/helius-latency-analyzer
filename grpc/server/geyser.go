package main

import (
	context "context"
	"solgrpc/proto"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

type GeyserServer struct {
	proto.UnimplementedGeyserServer
}

func (GeyserServer) Subscribe(grpc.BidiStreamingServer[proto.SubscribeRequest, proto.SubscribeUpdate]) error {
	return status.Errorf(codes.Unimplemented, "method Subscribe not implemented")
}
func (GeyserServer) Ping(context.Context, *proto.PingRequest) (*proto.PongResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Ping not implemented")
}
func (GeyserServer) GetLatestBlockhash(context.Context, *proto.GetLatestBlockhashRequest) (*proto.GetLatestBlockhashResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetLatestBlockhash not implemented")
}
func (GeyserServer) GetBlockHeight(context.Context, *proto.GetBlockHeightRequest) (*proto.GetBlockHeightResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetBlockHeight not implemented")
}
func (GeyserServer) GetSlot(context.Context, *proto.GetSlotRequest) (*proto.GetSlotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSlot not implemented")
}
func (GeyserServer) IsBlockhashValid(context.Context, *proto.IsBlockhashValidRequest) (*proto.IsBlockhashValidResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method IsBlockhashValid not implemented")
}
func (GeyserServer) GetVersion(context.Context, *proto.GetVersionRequest) (*proto.GetVersionResponse, error) {
	return &proto.GetVersionResponse{Version: "v1.0.1"}, nil
}
