package model

//组件的一些通用结构变量

type RequestEntity struct {
	//组件名称
	ComponentId string `json:"component_name" form:"component_name" binding:"required"`
	//组件入参
	Request map[string]interface{} `json:"request_data" form:"request_data" binding:"required"`
}
