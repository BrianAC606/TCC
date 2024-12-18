package model

import "context"

type TCCReq struct {
	TXId        string                 `json:"tx_id" form:"tx_id" binding:"required"`
	Componentid string                 `json:"component_id" form:"component_id" binding:"required"`
	RequestArg  map[string]interface{} `json:"request_arg" form:"request_arg" binding:"required"`
}

type TCCResp struct {
	TXId        string `json:"tx_id" form:"tx_id" binding:"required"`
	Componentid string `json:"component_id" form:"component_id" binding:"required"`
	ACK         bool   `json:"ack" form:"ack" binding:"required"`
}

type TCCComponent interface {
	ID() string
	Try(context.Context, *TCCReq) (*TCCResp, error)
	Confirm(context.Context, string) (*TCCResp, error)
	Cancel(context.Context, string) (*TCCResp, error)
}
