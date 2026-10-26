package service

import (
	"context"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/store"
)

func (s Service) SuspendRecovery(ctx context.Context, caseID string, stops []store.RecoveryStop) error {
	return s.Repo.SuspendRecovery(ctx, caseID, stops)
}
