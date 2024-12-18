package third_party

const (
	// 默认连接数超过10s后释放连接
	DefaultIdleTimeoutSeconds = 10
	// 默认最大连接数
	DefaultMaxConnection = 100
	// 默认最大空闲连接数
	DefaultMaxIdleConnection = 20
)

type ClientOptions struct {
	//基本参数
	network  string
	address  string
	password string

	//额外参数
	maxIdle            int
	idleTimeoutSeconds int
	maxConnection      int
	wait               bool
}

type ClientOption func(c *ClientOptions)

func WithMaxIdle(maxIdle int) ClientOption {
	return func(c *ClientOptions) {
		c.maxIdle = maxIdle
	}
}

func WithIdleTimeoutSeconds(idleTimeoutSeconds int) ClientOption {
	return func(c *ClientOptions) {
		c.idleTimeoutSeconds = idleTimeoutSeconds
	}
}

func WithMaxConnection(maxConnection int) ClientOption {
	return func(c *ClientOptions) {
		c.maxConnection = maxConnection
	}
}

func WithWaitMode() ClientOption {
	return func(c *ClientOptions) {
		c.wait = true
	}
}

func repairClientOpt(c *ClientOptions) {
	if c.maxIdle < 0 {
		c.maxIdle = DefaultMaxIdleConnection
	}
	if c.maxConnection < 0 {
		c.maxConnection = DefaultMaxConnection
	}
	if c.idleTimeoutSeconds < 0 {
		c.idleTimeoutSeconds = DefaultIdleTimeoutSeconds
	}
}
