package utils

import (
	"sync"

	"github.com/unimap-icp-hunter/project/internal/model"
)

// AssetPool 统一资产对象池
type AssetPool struct {
	pool sync.Pool
}

// NewAssetPool 创建资产对象池
func NewAssetPool() *AssetPool {
	return &AssetPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &model.UnifiedAsset{
					Headers: make(map[string]string),
					Extra:   make(map[string]interface{}),
				}
			},
		},
	}
}

// Get 从池中获取资产对象
func (p *AssetPool) Get() *model.UnifiedAsset {
	asset := p.pool.Get().(*model.UnifiedAsset)
	// 重置对象字段
	p.resetAsset(asset)
	return asset
}

// Put 将资产对象放回池中
func (p *AssetPool) Put(asset *model.UnifiedAsset) {
	if asset != nil {
		p.pool.Put(asset)
	}
}

// resetAsset 重置资产对象字段
func (p *AssetPool) resetAsset(asset *model.UnifiedAsset) {
	asset.IP = ""
	asset.Port = 0
	asset.Protocol = ""
	asset.Host = ""
	asset.URL = ""
	asset.Title = ""
	asset.BodySnippet = ""
	asset.Server = ""
	asset.StatusCode = 0
	asset.CountryCode = ""
	asset.Region = ""
	asset.City = ""
	asset.ASN = ""
	asset.Org = ""
	asset.ISP = ""
	asset.Source = ""

	// 清空映射
	for k := range asset.Headers {
		delete(asset.Headers, k)
	}

	for k := range asset.Extra {
		delete(asset.Extra, k)
	}
}

// SlicePool 切片对象池
type SlicePool struct {
	pool sync.Pool
}

// NewSlicePool 创建切片对象池
func NewSlicePool() *SlicePool {
	return &SlicePool{
		pool: sync.Pool{
			New: func() interface{} {
				return &[]model.UnifiedAsset{}
			},
		},
	}
}

// Get 从池中获取切片
func (p *SlicePool) Get() *[]model.UnifiedAsset {
	slice := p.pool.Get().(*[]model.UnifiedAsset)
	// 清空切片
	*slice = (*slice)[:0]
	return slice
}

// Put 将切片放回池中
func (p *SlicePool) Put(slice *[]model.UnifiedAsset) {
	if slice != nil {
		p.pool.Put(slice)
	}
}

// MapPool 映射对象池
type MapPool struct {
	pool sync.Pool
}

// NewMapPool 创建映射对象池
func NewMapPool() *MapPool {
	return &MapPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make(map[string]string)
			},
		},
	}
}

// Get 从池中获取映射
func (p *MapPool) Get() map[string]string {
	m := p.pool.Get().(map[string]string)
	// 清空映射
	for k := range m {
		delete(m, k)
	}
	return m
}

// Put 将映射放回池中
func (p *MapPool) Put(m map[string]string) {
	if m != nil {
		p.pool.Put(m)
	}
}

// InterfaceMapPool 接口映射对象池
type InterfaceMapPool struct {
	pool sync.Pool
}

// NewInterfaceMapPool 创建接口映射对象池
func NewInterfaceMapPool() *InterfaceMapPool {
	return &InterfaceMapPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make(map[string]interface{})
			},
		},
	}
}

// Get 从池中获取接口映射
func (p *InterfaceMapPool) Get() map[string]interface{} {
	m := p.pool.Get().(map[string]interface{})
	// 清空映射
	for k := range m {
		delete(m, k)
	}
	return m
}

// Put 将接口映射放回池中
func (p *InterfaceMapPool) Put(m map[string]interface{}) {
	if m != nil {
		p.pool.Put(m)
	}
}
