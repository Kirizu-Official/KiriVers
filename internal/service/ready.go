package service

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/database"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

// CheckReady 在 DB Ping 与各存储 Head 探针都成功时返回 nil，供 GET /health 的 ready 字段使用。
// 所有传入 Backend 都会探测：任一失败即非就绪（AND）。stores 为空视同存储不可用。
func CheckReady(ctx context.Context, db *gorm.DB, stores ...storage.Backend) error {
	var errs []error
	if err := database.Ping(ctx, db); err != nil {
		errs = append(errs, fmt.Errorf("database: %w", err))
	}
	if len(stores) == 0 {
		errs = append(errs, errors.New("storage is nil"))
	}
	for i, store := range stores {
		if store == nil {
			errs = append(errs, errors.New("storage is nil"))
			continue
		}
		if _, _, err := store.Head(ctx, storage.ReadyProbeKey); err != nil {
			if i == 0 {
				errs = append(errs, fmt.Errorf("storage: %w", err))
			} else {
				errs = append(errs, fmt.Errorf("private storage: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}
