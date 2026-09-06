package utils

import (
	"reflect"
	"time"

	"github.com/unimap/project/internal/model"
)

// Cache snapshots preserve dynamic Extra types. Unsupported opaque values,
// cycles/deep graphs, and oversized graphs bypass this optional cache instead
// of retaining caller-owned references. Callers must not mutate during Set.
func cloneCacheAssets(assets []model.UnifiedAsset) ([]model.UnifiedAsset, bool) {
	if assets == nil {
		return nil, true
	}
	budget := 100000
	value, ok := cloneCacheValue(reflect.ValueOf(assets), 0, &budget)
	if !ok {
		return nil, false
	}
	snapshot, valid := value.Interface().([]model.UnifiedAsset)
	return snapshot, valid
}

var cacheTimeType = reflect.TypeOf(time.Time{})

func cloneCacheValue(v reflect.Value, depth int, budget *int) (reflect.Value, bool) {
	if depth > 64 || *budget <= 0 {
		return reflect.Value{}, false
	}
	*budget--
	switch v.Kind() {
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return v, true
	case reflect.Interface, reflect.Ptr:
		if v.IsNil() {
			return reflect.Zero(v.Type()), true
		}
		inner, ok := cloneCacheValue(v.Elem(), depth+1, budget)
		if !ok {
			return reflect.Value{}, false
		}
		if v.Kind() == reflect.Ptr {
			out := reflect.New(v.Type().Elem())
			out.Elem().Set(inner)
			return out.Convert(v.Type()), true
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(inner)
		return out, true
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return reflect.Zero(v.Type()), true
		}
		if v.Len() > *budget {
			return reflect.Value{}, false
		}
		var out reflect.Value
		if v.Kind() == reflect.Slice {
			out = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		} else {
			out = reflect.New(v.Type()).Elem()
		}
		for i := 0; i < v.Len(); i++ {
			item, ok := cloneCacheValue(v.Index(i), depth+1, budget)
			if !ok {
				return reflect.Value{}, false
			}
			out.Index(i).Set(item)
		}
		return out, true
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return reflect.Value{}, false
		}
		if v.IsNil() {
			return reflect.Zero(v.Type()), true
		}
		if v.Len() > *budget {
			return reflect.Value{}, false
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			item, ok := cloneCacheValue(iter.Value(), depth+1, budget)
			if !ok {
				return reflect.Value{}, false
			}
			out.SetMapIndex(iter.Key(), item)
		}
		return out, true
	case reflect.Struct:
		// time.Time is an immutable value by API; copying preserves location and
		// monotonic time without inspecting unexported implementation fields.
		if v.Type() == cacheTimeType {
			return v, true
		}
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath != "" {
				return reflect.Value{}, false
			}
			field, ok := cloneCacheValue(v.Field(i), depth+1, budget)
			if !ok {
				return reflect.Value{}, false
			}
			out.Field(i).Set(field)
		}
		return out, true
	default:
		return reflect.Value{}, false
	}
}
