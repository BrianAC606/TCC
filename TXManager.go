package TCC

import (
	"TCC/internel"
	"TCC/model"
	"TCC/pkg"
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type Options struct {
	Timeout     time.Duration
	MonitorTick time.Duration
}

type TXManager struct {
	ctx            context.Context
	stop           context.CancelFunc
	opts           *Options                 //额外参数
	txStore        model.TXStore            //事务储存中心
	registryCenter *internel.RegistryCenter //注册中心
}

// 组件的实体
type ComponentEntity struct {
	Component model.TCCComponent     //对组件实体的基本操作函数
	Request   map[string]interface{} //请求参数
}

type Option func(opts *Options)

func NewTXManager(txStore model.TXStore, opts ...Option) *TXManager {
	ctx, cancel := context.WithCancel(context.Background())
	tm := &TXManager{
		ctx:            ctx,
		stop:           cancel,
		opts:           &Options{},
		txStore:        txStore,
		registryCenter: internel.NewRegistryCenter(),
	}
	for _, opt := range opts {
		opt(tm.opts)
	}

	checkOpt(tm.opts)

	go tm.polling()

	return tm
}

func (tm *TXManager) Transaction(ctx context.Context, reqs ...*model.RequestEntity) (bool, error) {
	//1.限制分布式事务的执行时长
	ctx, cancel := context.WithTimeout(tm.ctx, tm.opts.Timeout)
	defer cancel()
	//2.获取所有的TCC组件
	componententities, err := tm.getcomponents(ctx, reqs...)
	if err != nil {
		return false, err
	}
	//3.创建事务
	TXId, err := tm.txStore.CreateTX(ctx, toTCCComponents(componententities)...)
	if err != nil {
		return false, err
	}
	//开启两阶段
	return tm.TwoPhaseCommit(ctx, TXId, componententities)
}

// 完成事务的第一阶段提交: try
func (tm *TXManager) TwoPhaseCommit(ctx context.Context, TXId string, componentEnities []ComponentEntity) (bool, error) {
	//对两阶段提交的上下文控制
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	//创建一个error channel能够在任何try失败时即使取得消息
	errchan := make(chan error)

	//向所有组件发送try请求
	go func() {
		wg := sync.WaitGroup{}
		for _, componentEntity := range componentEnities {
			wg.Add(1)
			go func() {
				defer wg.Done() //defer保证了即使某些try请求发生错误也能执行Done(),保证waitgroup不会阻塞
				resp, err := componentEntity.Component.Try(cctx, &model.TCCReq{
					TXId:        TXId,
					Componentid: componentEntity.Component.ID(),
					RequestArg:  componentEntity.Request,
				})
				if err != nil && !resp.ACK {
					_ = tm.txStore.TXUpdate(cctx, TXId, componentEntity.Component.ID(), false) //try失败,更新component的状态
					errchan <- fmt.Errorf("component:%v update failed: %v", componentEntity.Component.ID(), err)
					return
				}
				//try成功，更新component的状态
				if err := tm.txStore.TXUpdate(cctx, TXId, componentEntity.Component.ID(), true); err != nil {
					errchan <- fmt.Errorf("component:%v update failed: %v", componentEntity.Component.ID(), err)
				}
			}()
		}
		wg.Wait()
		close(errchan)
	}()

	successful := true
	if err := <-errchan; err != nil {
		cancel()
		successful = false //不能直接返回，因为还需要推进事务，对所有try逐渐进行cancel
	}

	go func() {
		err := tm.advanceProgressByTXId(TXId)
		if err != nil {
			log.Println("advanceProgressByTXId:", err)
		}
	}()
	return successful, nil
}

// 根据事务id取出当前事务
func (tm *TXManager) advanceProgressByTXId(TXId string) error {
	tx, err := tm.txStore.GetTX(tm.ctx, TXId)
	if err != nil {
		return err
	}
	return tm.advanceProgress(tx)
}

// 根据事务完成所有组件的第二次提交和事务的最终提交
func (tm *TXManager) advanceProgress(tx pkg.Transaction) error {
	txstatus := tx.GetStatus(time.Now().Add(-tm.opts.MonitorTick))
	if txstatus == pkg.TXHanging {
		return nil //事务悬挂则不处理
	}

	success := txstatus == pkg.TXSuccess
	var cancelOrCommit func(ctx context.Context, component model.TCCComponent) (*model.TCCResp, error)
	var TXcommit func(ctx context.Context) error
	if success {
		//组件的第二次commit: confirm
		cancelOrCommit = func(ctx context.Context, component model.TCCComponent) (*model.TCCResp, error) {
			return component.Confirm(ctx, component.ID())
		}

		//事务的最终提交: 成功
		TXcommit = func(ctx context.Context) error {
			return tm.txStore.TXSubmit(ctx, tx.TXid, true)
		}
	} else {
		//组件的第二次 commit: cancel
		cancelOrCommit = func(ctx context.Context, component model.TCCComponent) (*model.TCCResp, error) {
			return component.Cancel(ctx, component.ID())
		}

		TXcommit = func(ctx context.Context) error {
			return tm.txStore.TXSubmit(ctx, tx.TXid, false)
		}
	}

	//遍历各个组件，完成第二次提交
	for _, entity := range tx.ComponentsStatus {
		component, err := tm.registryCenter.GetComponentByIDs(entity.ComponentId)
		if err != nil {
			return err
		}
		Resp, err := cancelOrCommit(tm.ctx, component[0])
		if err != nil {
			return err
		}
		if Resp.ACK == false {
			return fmt.Errorf("component:%v has not ACK", entity.ComponentId)
		}
	}

	return TXcommit(tm.ctx)
}

// 轮询: try成功后会不断轮询进行二阶段提交，保证各个组件的cancelOrCommit行为能够有效执行，不会因为网络波动而影响最终执行结果
func (tm *TXManager) polling() {
	var err error
	var tick time.Duration
	for {
		if err == nil {
			tick = tm.opts.MonitorTick
		} else {
			tick = tm.backOffTick(tm.opts.MonitorTick)
		}

		select {
		case <-tm.ctx.Done():
			return
		case <-time.After(tick):
			if err := tm.txStore.Lock(tm.ctx, tm.opts.MonitorTick); err != nil {
				continue
			}
			var txs []*pkg.Transaction
			txs, err = tm.txStore.GetHangingTXs(tm.ctx)
			if err != nil {
				_ = tm.txStore.Unlock(tm.ctx)
				continue
			}
			err = tm.ReCommitAllTransaction(txs)
			_ = tm.txStore.Unlock(tm.ctx)
		}
	}
}

// 对选中的所有事务进行二阶段提交
func (tm *TXManager) ReCommitAllTransaction(txs []*pkg.Transaction) error {
	errchan := make(chan error)
	go func() {
		wg := sync.WaitGroup{}
		for _, tx := range txs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := tm.advanceProgress(*tx); err != nil {
					errchan <- err
				}
			}()
		}
		wg.Wait()
		close(errchan)
	}()

	var firstErr error
	for err := range errchan {
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ---------------------------------------------------------------------------------------------------------------
// ----------------------------------------------TXmanager非核心函数------------------------------------------------
// ---------------------------------------------------------------------------------------------------------------

// TXmanager 的一些工具成员函数
func (tm *TXManager) getcomponents(ctx context.Context, reqs ...*model.RequestEntity) (componententities []ComponentEntity, err error) {
	for _, req := range reqs {
		cp, err := tm.registryCenter.GetComponentByIDs(req.ComponentId)
		if err != nil {
			return nil, err
		}
		componententities = append(componententities, ComponentEntity{
			Component: cp[0],
			Request:   req.Request,
		})
	}
	return componententities, nil
}

func (tm TXManager) backOffTick(tick time.Duration) time.Duration {
	maxTick := tm.opts.MonitorTick << 3
	tick <<= 1
	if tick > maxTick {
		tick = maxTick
	}
	return tick
}

// TXManager 的一些辅助处理函数
func toTCCComponents(componententities []ComponentEntity) []model.TCCComponent {
	components := make([]model.TCCComponent, len(componententities))
	for i, component := range componententities {
		components[i] = component.Component
	}
	return components
}

// 检查option参数是否合法
func checkOpt(opts *Options) {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.MonitorTick <= 0 {
		opts.MonitorTick = 10 * time.Second
	}
}
