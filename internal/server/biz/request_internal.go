package biz

import (
	"context"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
)

func (s *RequestService) getDataStorage(ctx context.Context, dataStorageID int) (*ent.DataStorage, error) {
	ctx = authz.WithSystemBypass(ctx, "request-get-datastorage")
	if dataStorageID == 0 {
		return s.DataStorageService.GetPrimaryDataStorage(ctx)
	}

	dataStorage, err := s.DataStorageService.GetDataStorageByID(ctx, dataStorageID)
	if err != nil {
		return nil, err
	}

	return dataStorage, nil
}
