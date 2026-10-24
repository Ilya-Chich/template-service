package postgres

import (
	"context"
	"errors"
	"fmt"

	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/port"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type dummyModel struct {
	ID   string `gorm:"column:id;type:uuid;primaryKey"`
	Name string `gorm:"column:name"`
	Note string `gorm:"column:note"`
}

func (dummyModel) TableName() string {
	return "dummies"
}

type Dummy struct {
	db *gorm.DB
}

var _ port.DummyRepository = (*Dummy)(nil)

func NewDummy(db *gorm.DB) *Dummy {
	return &Dummy{db: db}
}

func (r *Dummy) Get(ctx context.Context, id string) (domain.Dummy, error) {
	var row dummyModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return domain.Dummy{}, mapError(err)
	}
	return domain.Dummy{ID: row.ID, Name: row.Name, Note: row.Note}, nil
}

func (r *Dummy) GetByName(ctx context.Context, name string) (domain.Dummy, error) {
	var row dummyModel
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&row).Error; err != nil {
		return domain.Dummy{}, mapError(err)
	}
	return domain.Dummy{ID: row.ID, Name: row.Name, Note: row.Note}, nil
}

func (r *Dummy) Insert(ctx context.Context, dummy domain.Dummy) error {
	row := dummyModel{ID: dummy.ID, Name: dummy.Name, Note: dummy.Note}
	return mapError(r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error)
}

func (r *Dummy) Update(ctx context.Context, id string, patch domain.Patch) error {
	if err := patch.Validate(); err != nil {
		return err
	}
	updates := make(map[string]any)
	if patch.Name != nil {
		updates["name"] = *patch.Name
	}
	if patch.Note != nil {
		updates["note"] = *patch.Note
	}
	return mapError(r.db.WithContext(ctx).Model(&dummyModel{}).Where("id = ?", id).Updates(updates).Error)
}

func (r *Dummy) Delete(ctx context.Context, id string) error {
	return mapError(r.db.WithContext(ctx).Where("id = ?", id).Delete(&dummyModel{}).Error)
}

func mapError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %w", domain.ErrNotFound, err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %w", domain.ErrConflict, err)
		case "23514":
			return fmt.Errorf("%w: %w", domain.ErrInvalid, err)
		}
	}
	return err
}
