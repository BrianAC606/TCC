package internel

import (
	"TCC/DAO"
	"TCC/model"
	"TCC/pkg"
	"TCC/redis_lock"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/demdxx/gocast"
)

type MockTXStore struct {
	dao DAO.TXRecordDAOInterface

	lock *redis_lock.RedisLock
}

func (m *MockTXStore) CreateTX(ctx context.Context, components ...model.TCCComponent) (string, error) {
	componentTryStatuses := make(map[string]*DAO.ComponentTryStatus, len(components))

	for _, component := range components {
		componentTryStatuses[component.ID()] = &DAO.ComponentTryStatus{
			ComponentID: component.ID(),
			TryStatus:   pkg.TryHanging.String(),
		}
	}

	body, _ := json.Marshal(componentTryStatuses)
	txID, err := m.dao.CreateTXRecord(ctx, &DAO.TXRecordPO{
		Status:               pkg.TryHanging.String(),
		ComponentTryStatuses: string(body),
	})
	if err != nil {
		return "", err
	}
	return gocast.ToString(txID), nil
}

func (m *MockTXStore) TXUpdate(ctx context.Context, TXId string, componentId string, successful bool) error {
	txId := gocast.ToUint(TXId)
	status := pkg.TryFailure.String()
	if successful {
		status = pkg.TrySuccess.String()
	}

	return m.dao.UpdateComponentStatus(ctx, txId, componentId, status)
}

func (m *MockTXStore) TXSubmit(ctx context.Context, TXId string, success bool) error {
	do := func(ctx context.Context, dao *DAO.TXRecordDAO, record *DAO.TXRecordPO) error {
		if success {
			if record.Status == pkg.TryFailure.String() {
				return fmt.Errorf("invalid TX status: %s, txid: %s", record.Status, TXId)
			}
			record.Status = pkg.TrySuccess.String()
		} else {
			if record.Status == pkg.TrySuccess.String() {
				return fmt.Errorf("invalid TX status: %s, txid: %s", record.Status, TXId)
			}
			record.Status = pkg.TryFailure.String()
		}
		return dao.UpdateTXRecord(ctx, record)
	}
	return m.dao.LockAndDo(ctx, gocast.ToUint(TXId), do)
}

func (m *MockTXStore) GetHangingTXs(ctx context.Context) ([]*pkg.Transaction, error) {
	records, err := m.dao.GetTXRecords(ctx, DAO.WithStatus(pkg.TryHanging))
	if err != nil {
		return nil, err
	}

	txs := make([]*pkg.Transaction, 0, len(records))
	for _, record := range records {
		componentTryStatuses := make(map[string]*DAO.ComponentTryStatus)
		_ = json.Unmarshal([]byte(record.ComponentTryStatuses), &componentTryStatuses)
		components := make([]*pkg.ComponentTryEntity, 0, len(componentTryStatuses))
		for _, component := range componentTryStatuses {
			components = append(components, &pkg.ComponentTryEntity{
				ComponentId:     component.ComponentID,
				ComponentStatus: pkg.ComponentTryStatus(component.TryStatus),
			})
		}
		txs = append(txs, &pkg.Transaction{
			TXid:             gocast.ToString(record.ID),
			ComponentsStatus: components,
			TxStatus:         pkg.TXHanging,
			CreatedAt:        record.CreatedAt,
		})
	}
	return txs, nil
}

func (m *MockTXStore) GetTX(ctx context.Context, TXId string) (*pkg.Transaction, error) {
	record, err := m.dao.GetTXRecords(ctx, DAO.WithID(gocast.ToUint(TXId)))
	if err != nil {
		return nil, err
	}

	//每个组件的id和状态
	componentTryStatuses := make(map[string]*DAO.ComponentTryStatus)
	json.Unmarshal([]byte(record[0].ComponentTryStatuses), &componentTryStatuses)
	components := make([]*pkg.ComponentTryEntity, 0, len(componentTryStatuses))
	for _, component := range componentTryStatuses {
		components = append(components, &pkg.ComponentTryEntity{
			ComponentId:     component.ComponentID,
			ComponentStatus: pkg.ComponentTryStatus(component.TryStatus),
		})
	}
	return &pkg.Transaction{
		TXid:             TXId,
		ComponentsStatus: components,
		TxStatus:         pkg.TXStatus(record[0].Status),
		CreatedAt:        record[0].CreatedAt,
	}, nil
}

func (m *MockTXStore) Lock(ctx context.Context, duration time.Duration) error {
	err := m.lock.Lock(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (m *MockTXStore) Unlock(ctx context.Context) error {
	err := m.lock.Unlock(ctx)
	if err != nil {
		return err
	}
	return nil
}
