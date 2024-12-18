package redis_lock

import (
	"TCC/third_party"
	"context"
	"errors"
	"fmt"
	"github.com/gomodule/redigo/redis"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const RedisLockKeyPrePrefix = "REDIS_LOCK_PREFIX"

type RedisLock struct {
	key    string
	token  string
	client third_party.LockClient

	LockOptions

	//看门狗运作标识
	runningDog int32
	//停止看门狗
	stopDog context.CancelFunc
}

func NewRedisLock(key string, client third_party.LockClient, opts ...LockOption) *RedisLock {
	r := &RedisLock{
		key:    key,
		client: client,
		token:  getPidAndGidStr(),
	}

	for _, opt := range opts {
		opt(&r.LockOptions)
	}

	repairLockOpt(&r.LockOptions)

	return r
}

func (r *RedisLock) Lock(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			return
		}

		//在加锁成功下启动看门狗模式
		r.startWatchDog(ctx)
	}()

	//先尝试取锁
	err = r.tryLock(ctx)
	if err == nil {
		//取锁成功则直接返回
		return nil
	}

	if !r.isBlock {
		//如果不是非阻塞模式则直接返回错误
		return err
	}

	if !IsRetryableErr(err) {
		//如果错误不可重试，则也会直接返回错误
		return err
	}

	//错误可重试, 即抢锁失败的情况下会进入轮询阻塞状态
	return r.blockingLock(ctx)
}

func (r *RedisLock) tryLock(ctx context.Context) error {
	resp, err := r.client.SetNXWithEX(ctx, r.getLockKey(), r.token, r.expireSeconds)
	if err != nil {
		return err
	}
	if resp != 1 {
		// %w是嵌套ErrLockInUse进一个新的error, ErrLockInUse属于这个新error
		return fmt.Errorf("reply: %d, err: %w", resp, ErrLockInUse)
	}

	//取锁成功
	return nil
}

func (r *RedisLock) startWatchDog(ctx context.Context) {
	//没启用看门狗模式则直接返回
	if !r.watchDogMode {
		return
	}

	for !atomic.CompareAndSwapInt32(&r.runningDog, 0, 1) {
		time.Sleep(time.Second) //循环等待之前的看门狗回事
	}

	ctx, r.stopDog = context.WithCancel(ctx)

	//进入看门狗模式
	go func() {
		defer func() {
			atomic.StoreInt32(&r.runningDog, 0)
		}()
		r.watchDogRunning(ctx)
	}()
}

func (r *RedisLock) watchDogRunning(ctx context.Context) {
	ticker := time.NewTicker(WatchDogWorkStepSeconds * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		select {
		case <-ctx.Done():
			return
		default:
			//给锁续期，为了避免网络阻塞造成时延，续期时间会额外增加5s
			err := r.DelayExpire(ctx, WatchDogWorkStepSeconds+5)
			if err != nil {
				log.Printf("redis_lock: watchDogRunning err:%v", err)
			}
		}
	}
}

// 更新锁的过期时间。
// 基于lua脚本实现原子性操作
func (r *RedisLock) DelayExpire(ctx context.Context, expireSeconds int64) error {
	keysAndArgs := []interface{}{r.getLockKey(), r.token, expireSeconds}

	reply, err := r.client.Eval(ctx, third_party.LuaCheckAndExpireDistributionLock, 1, keysAndArgs)

	if ret, _ := reply.(int64); ret != 1 {
		return fmt.Errorf("fail to delay expired key:%s expire:%d err: %w", r.getLockKey(), expireSeconds, err)
	}
	return nil
}

func (r *RedisLock) blockingLock(ctx context.Context) error {
	timeoutCh := time.After(time.Duration(r.blockWaitingSeconds) * time.Second)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		select {
		case <-ctx.Done():
			return fmt.Errorf("ctx timeout, lock fai: %w", ctx.Err())
		case <-timeoutCh:
			return fmt.Errorf("block wait timeout: %w", ctx.Err())
		default:
			err := r.tryLock(ctx)

			if err == nil {
				return nil //成功加锁后返回
			}

			//非重试错误，直接返回
			if !IsRetryableErr(err) {
				return err
			}
		}
	}

	return nil
}

func (r *RedisLock) Unlock(ctx context.Context) error {
	defer func() {
		if r.stopDog != nil {
			r.stopDog()
		}
	}()

	keysAndArgs := []interface{}{r.getLockKey(), r.token}

	resp, err := r.client.Eval(ctx, third_party.LuaCheckAndDeleteDistributionLock, 1, keysAndArgs)

	if ret, _ := resp.(int64); ret != 1 {
		return fmt.Errorf("fail to unlock key:%s err: %w", r.getLockKey(), err)
	}

	return nil
}

func (r *RedisLock) getLockKey() string {
	return RedisLockKeyPrePrefix + r.key
}

//------------------------------------------------------------
//---------------------------tool函数--------------------------
//------------------------------------------------------------

var (
	ErrLockInUse = errors.New("lock already acquired by other")
	ErrNil       = redis.ErrNil
)

// 判断当前错误是否由锁阻塞引起的
func IsRetryableErr(err error) bool {
	return errors.Is(err, ErrLockInUse)
}

// 获取当前进程Id
func getCurrentProcessID() string {
	return strconv.Itoa(os.Getpid())
}

func getCurrentGoroutineID() string {
	buf := make([]byte, 128)
	buf = buf[:runtime.Stack(buf, false)]
	stacktInfo := string(buf)
	return strings.TrimSpace(strings.Split(strings.Split(stacktInfo, "[running]")[0], "goroutine")[1])
}

func getPidAndGidStr() string {
	return fmt.Sprintf("%s-%s", getCurrentProcessID(), getCurrentGoroutineID())
}
