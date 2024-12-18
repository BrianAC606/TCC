package pkg

import "fmt"

func BuildTXKey(componentId, txId string) string {
	return fmt.Sprintf("TX_key:%s_%s", txId, componentId)
}

func BuildTXKeyWithDetail(componentId, txId string) string {
	return fmt.Sprintf("TX_detail_key:%s_%s", componentId, txId)
}

func BuildRedisLockKey(componentId, txId string) string {
	return fmt.Sprintf("TX_lock_key:%s_%s", txId, componentId)
}

func BuildDataKey(componentId, txId, bizid string) string {
	return fmt.Sprintf("DATA_key:%s_%s_%s", txId, componentId, bizid)
}
