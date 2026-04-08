package shipping

import (
	"context"
	"fmt"

	pb "shipping-service/proto"

	"github.com/sirupsen/logrus"
)

// Service implements the gRPC ShippingService.
type Service struct {
	pb.UnimplementedShippingServiceServer
	log *logrus.Logger
}

// NewService creates a new shipping service.
func NewService(log *logrus.Logger) *Service {
	return &Service{log: log}
}

// GetQuote produces a shipping quote (cost) in USD.
func (s *Service) GetQuote(_ context.Context, req *pb.GetQuoteRequest) (*pb.GetQuoteResponse, error) {
	s.log.Info("[GetQuote] received request")
	defer s.log.Info("[GetQuote] completed request")

	count := 0
	for _, item := range req.Items {
		count += int(item.Quantity)
	}
	quote := QuoteFromCount(count)

	return &pb.GetQuoteResponse{
		CostUsd: &pb.Money{
			CurrencyCode: "USD",
			Units:        int64(quote.Dollars),
			Nanos:        int32(quote.Cents * 10_000_000), //#nosec G115  // Cents is always 0-99, overflow impossible
		},
	}, nil
}

// ShipOrder mocks shipping the requested items and returns a tracking ID.
func (s *Service) ShipOrder(_ context.Context, req *pb.ShipOrderRequest) (*pb.ShipOrderResponse, error) {
	s.log.Info("[ShipOrder] received request")
	defer s.log.Info("[ShipOrder] completed request")

	baseAddress := fmt.Sprintf("%s, %s, %s", req.Address.StreetAddress, req.Address.City, req.Address.State)
	id := TrackingID(baseAddress)

	return &pb.ShipOrderResponse{TrackingId: id}, nil
}
