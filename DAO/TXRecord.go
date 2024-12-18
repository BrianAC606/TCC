package DAO

import (
	"TCC/pkg"
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TXRecordDAOInterface interface {
	GetTXRecords(ctx context.Context, opts ...QueryOption) ([]*TXRecordPO, error)
	CreateTXRecord(ctx context.Context, record *TXRecordPO) (uint, error)
	UpdateTXRecord(ctx context.Context, record *TXRecordPO) error
	UpdateComponentStatus(ctx context.Context, id uint, componentID string, Status string) error
	LockAndDo(ctx context.Context, id uint, do func(ctx context.Context, dao *TXRecordDAO, record *TXRecordPO) error) error
}

type TXRecordPO struct {
	gorm.Model
	Status               string `gorm:"status"`
	ComponentTryStatuses string `gorm:"component_try_statuses"`
}

func (t TXRecordPO) TableName() string {
	return "TXRecordPO"
}

type ComponentTryStatus struct {
	ComponentID string `json:"component_id"`
	TryStatus   string `json:"try_status"`
}

type TXRecordDAO struct {
	db *gorm.DB
}

func NewTXRecordDAO(db *gorm.DB) *TXRecordDAO {
	return &TXRecordDAO{
		db: db,
	}
}

func (dao *TXRecordDAO) GetTXRecords(ctx context.Context, opts ...QueryOption) ([]*TXRecordPO, error) {
	var records []*TXRecordPO
	db := dao.db.WithContext(context.Background()).Model(&TXRecordPO{})

	for _, opt := range opts {
		db = opt(db)
	}

	return records, db.Find(&records).Error
}

func (dao *TXRecordDAO) CreateTXRecord(ctx context.Context, record *TXRecordPO) (uint, error) {
	return record.ID, dao.db.WithContext(ctx).Create(record).Error
}

func (dao *TXRecordDAO) UpdateTXRecord(ctx context.Context, record *TXRecordPO) error {
	return dao.db.WithContext(ctx).Model(&TXRecordPO{}).Updates(record).Error
}

// 更新组件的状态
// 如果状态已更新，则直接返回；如果状态不是tryHanging,则返回错误
func (dao *TXRecordDAO) UpdateComponentStatus(ctx context.Context, id uint, componentID string, status string) error {
	return dao.LockAndDo(ctx, id, func(ctx context.Context, dao *TXRecordDAO, record *TXRecordPO) error {
		var statuses map[string]*ComponentTryStatus
		if err := json.Unmarshal([]byte(record.ComponentTryStatuses), &statuses); err != nil {
			return err
		}
		componentStatus, ok := statuses[componentID]
		if !ok {
			return fmt.Errorf("component %s not exist in TX%d", componentID, id)
		}

		if componentStatus.TryStatus == status { //重复执行则直接跳过
			return nil
		}

		if componentStatus.TryStatus != pkg.TryHanging.String() {
			return fmt.Errorf("invalid status: %s of component: %s, txid: %d", componentStatus.TryStatus, componentID, id)
		}

		componentStatus.TryStatus = status
		body, _ := json.Marshal(componentStatus)
		record.ComponentTryStatuses = string(body)
		return dao.UpdateTXRecord(ctx, record)
	})
}

// 开启事务，并根据id查询对应的记录，然后根据记录执行do函数操作
func (dao *TXRecordDAO) LockAndDo(ctx context.Context, id uint, do func(ctx context.Context, dao *TXRecordDAO, record *TXRecordPO) error) error {
	return dao.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record := &TXRecordPO{}

		if err := dao.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(record, id).Error; err != nil {
			return err
		}

		Dao := NewTXRecordDAO(tx)
		return do(ctx, Dao, record)
	})
}
