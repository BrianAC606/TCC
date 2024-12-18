package redis_lock

const (
	// 默认分布式锁过期时间
	DefaultLockExpireSeconds = 30

	// 默认看门狗工作时间间隙
	WatchDogWorkStepSeconds = 10
)

type LockOptions struct {
	isBlock             bool
	blockWaitingSeconds int64
	expireSeconds       int64
	watchDogMode        bool
}

type LockOption func(c *LockOptions)

func WithBlock() LockOption {
	return func(c *LockOptions) {
		c.isBlock = true
	}
}

func WithBlockWaitingSeconds(blockWaitingSeconds int64) LockOption {
	return func(c *LockOptions) {
		c.blockWaitingSeconds = blockWaitingSeconds
	}
}

func WithExpireSeconds(expireSeconds int64) LockOption {
	return func(c *LockOptions) {
		c.expireSeconds = expireSeconds
	}
}

func repairLockOpt(c *LockOptions) {
	if c.isBlock && c.blockWaitingSeconds <= 0 {
		//默认阻塞等待时间为5秒
		c.blockWaitingSeconds = 5
	}

	if c.expireSeconds > 0 {
		return
	}

	//未设置过期时间，则启用看门狗模式
	c.expireSeconds = DefaultLockExpireSeconds
	c.watchDogMode = true
}
