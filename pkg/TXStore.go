package pkg

import "time"

//该文件主要记录组件和事务状态全局参数，以及单个事务变量

type ComponentTryStatus string

const (
	TryHanging ComponentTryStatus = "Hanging"
	TrySuccess ComponentTryStatus = "Success"
	TryFailure ComponentTryStatus = "Failure"
)

func (c ComponentTryStatus) String() string {
	return string(c)
}

type TXStatus string

const (
	TXHanging TXStatus = "Hanging"
	TXSuccess TXStatus = "Success"
	TXFailure TXStatus = "Failure"
)

func (c TXStatus) toString() string {
	return string(c)
}

type ComponentTryEntity struct {
	ComponentId     string
	ComponentStatus ComponentTryStatus
}

type Transaction struct {
	TXid             string                `json:"tx_id"`
	ComponentsStatus []*ComponentTryEntity `json:"component_try_entity"`
	TxStatus         TXStatus              `json:"tx_status"`
	CreatedAt        time.Time             `json:"created_at"`
}

func (t Transaction) GetStatus(createdBefore time.Time) TXStatus {
	var IsHanging bool
	for _, component := range t.ComponentsStatus {
		if component.ComponentStatus == TryFailure {
			return TXFailure
		}
		IsHanging = IsHanging || component.ComponentStatus == TryHanging
	}

	//超时则认为事务执行失败
	if IsHanging && t.CreatedAt.Before(createdBefore) {
		return TXFailure
	}

	//所有组件尚未Try完，事务悬挂
	if IsHanging {
		return TXHanging
	}

	return TXSuccess
}
