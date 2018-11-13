package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/noahhai/kube-vegeta/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type server struct{}

func (s *server) Compute(ctx context.Context, r *pb.GCDRequest) (*pb.GCDResponse, error) {
	a, b := r.A, r.B
	for b != 0 {
		a, b = b, a%b
	}
	return &pb.GCDResponse{Result: a}, nil
}

func main() {
	fmt.Println("starting")
	lis, err := net.Listen("tcp", ":3000")
	fmt.Println("Listening on :3000")
	if err != nil {
		log.Fatalf("Failed to start listening on :3000: $v", err)
	}
	s := grpc.NewServer()
	fmt.Println("registering gcd service")
	pb.RegisterGCDServiceServer(s, &server{})
	reflection.Register(s)
	fmt.Println("registered gcd service")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	} else {
		fmt.Println("Serving on :3000")
	}
}
