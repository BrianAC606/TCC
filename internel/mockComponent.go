package internel

import (
	"TCC/model"
	"TCC/pkg"
	"TCC/redis_lock"
	"TCC/third_party"
	"context"
	"errors"
	"fmt"
	"github.com/demdxx/gocast"
)

type ComponentStatus string

func (c ComponentStatus) String() string {
	return string(c)
}

const (
	TryStatus     ComponentStatus = "Try"
	ConfirmStatus ComponentStatus = "Confirm"
	CancelStatus  ComponentStatus = "Cancel"
)

type DataStatus string

func (ds DataStatus) String() string {
	return string(ds)
}

const (
	DataFrozen  DataStatus = "冻结态"
	DataSuccess DataStatus = "成功态"
)

type MockComponent struct {
	//组件的唯一标识id
	id string

	//分布式锁,保证组件数据访问一致性
	client *third_party.RedisClient
}

func NewMockComponent(id string, client *third_party.RedisClient) *MockComponent {
	return &MockComponent{id: id, client: client}
}

func (mc *MockComponent) ID() string {
	return mc.id
}

func (mc *MockComponent) Try(ctx context.Context, req *model.TCCReq) (*model.TCCResp, error) {
	//获取分布式锁
	lock := redis_lock.NewRedisLock(pkg.BuildRedisLockKey(mc.id, req.TXId), mc.client)
	err := lock.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = lock.Unlock(ctx)
	}()

	//幂等性获取事务
	CpStatus, err := mc.client.Get(ctx, pkg.BuildTXKey(mc.id, req.TXId))
	if err != nil && !errors.Is(err, redis_lock.ErrNil) {
		return nil, err
	}

	resp := &model.TCCResp{
		TXId:        req.TXId,
		Componentid: mc.id,
	}

	switch CpStatus {
	case TryStatus.String(), ConfirmStatus.String():
		resp.ACK = true
		return resp, nil
	case CancelStatus.String():
		return resp, nil
	default:
	}

	BizId := gocast.ToString(req.RequestArg["biz_id"])
	if _, err := mc.client.Set(ctx, pkg.BuildTXKeyWithDetail(mc.id, req.TXId), BizId); err != nil {
		return nil, err
	}

	reply, err := mc.client.SetNX(ctx, pkg.BuildDataKey(mc.id, req.TXId, BizId), DataFrozen.String())
	if err != nil {
		return nil, err
	}
	if reply != 1 {
		return resp, nil //重复设置数据状态则直接返回
	}

	if _, err := mc.client.Set(ctx, pkg.BuildTXKey(mc.id, req.TXId), TryStatus.String()); err != nil {
		return nil, err
	}

	resp.ACK = true
	return resp, nil
}

func (mc *MockComponent) Confirm(ctx context.Context, txid string) (*model.TCCResp, error) {
	lock := redis_lock.NewRedisLock(pkg.BuildRedisLockKey(mc.id, txid), mc.client)
	err := lock.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = lock.Unlock(ctx)
	}()

	resp := &model.TCCResp{
		TXId:        txid,
		Componentid: mc.id,
	}

	cpStatus, err := mc.client.Get(ctx, pkg.BuildTXKey(mc.id, txid))
	if err != nil {
		return nil, err
	}

	switch cpStatus {
	case ConfirmStatus.String():
		resp.ACK = true
		return resp, nil
	case CancelStatus.String():
		return resp, nil
	default:
	}

	bizId, err := mc.client.Get(ctx, pkg.BuildTXKeyWithDetail(mc.id, txid))
	if err != nil {
		return nil, err
	}

	dataStatus, err := mc.client.Get(ctx, pkg.BuildDataKey(mc.id, txid, bizId))
	if err != nil {
		return nil, err
	}

	if dataStatus != DataFrozen.String() {
		return resp, nil
	}

	if _, err := mc.client.Set(ctx, pkg.BuildDataKey(mc.id, txid, bizId), DataSuccess.String()); err != nil {
		return nil, err
	}

	_, _ = mc.client.Set(ctx, pkg.BuildTXKey(mc.id, txid), CancelStatus.String())

	resp.ACK = true
	return resp, nil
}

func (mc *MockComponent) Cancel(ctx context.Context, txid string) (*model.TCCResp, error) {
	lock := redis_lock.NewRedisLock(pkg.BuildRedisLockKey(mc.id, txid), mc.client)
	if err := lock.Lock(ctx); err != nil {
		return nil, err
	}
	defer func() {
		_ = lock.Unlock(ctx)
	}()

	cpStatus, err := mc.client.Get(ctx, pkg.BuildTXKey(mc.id, txid))
	if err != nil {
		return nil, err
	}

	if cpStatus == ConfirmStatus.String() {
		return nil, fmt.Errorf("invalid component status, cid: %s, txid: %s", mc.id, txid)
	}

	bizId, err := mc.client.Get(ctx, pkg.BuildTXKeyWithDetail(mc.id, txid))
	if err != nil {
		return nil, err
	}
	
	if err := mc.client.Del(ctx, pkg.BuildDataKey(mc.id, txid, bizId)); err != nil {
		return nil, err
	}

	_, _ = mc.client.Set(ctx, pkg.BuildTXKey(mc.id, txid), CancelStatus.String())

	return &model.TCCResp{
		TXId:        txid,
		Componentid: mc.id,
		ACK:         true,
	}, nil
}
