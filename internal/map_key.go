package internal

import (
	"fmt"
	"reflect"
	"strings"
)

var ifaceType = reflect.TypeOf((*interface{})(nil)).Elem()

type MapKey struct {
	iface interface{}
}

func NewMapKey(is []interface{}) MapKey {
	return MapKey{
		iface: newMapKey(is),
	}
}

func newMapKey(is []interface{}) interface{} {
	var sb strings.Builder
	for i, v := range is {
		if i > 0 {
			sb.WriteString("|")
		}
		sb.WriteString(fmt.Sprintf("%v", v))
	}
	return sb.String()
}
