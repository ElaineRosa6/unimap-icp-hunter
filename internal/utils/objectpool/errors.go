package objectpool

import "errors"

// 定义对象池错误
var (
	ErrPoolClosed   = errors.New("object pool is closed")
	ErrWaitTimeout  = errors.New("wait timeout when acquiring object")
	ErrInvalidObject = errors.New("invalid object")
)