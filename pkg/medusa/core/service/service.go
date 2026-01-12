package service

import (
	"github.com/imlargo/medusa/pkg/medusa/core/logger"
)

type Service struct {
	logger *logger.Logger
}

func NewService(
	logger *logger.Logger,
) *Service {
	return &Service{
		logger,
	}
}
