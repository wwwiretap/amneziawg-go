//go:build !linux

package device

import (
	"github.com/wwwiretap/amneziawg-go/conn"
	"github.com/wwwiretap/amneziawg-go/rwcancel"
)

func (device *Device) startRouteListener(bind conn.Bind) (*rwcancel.RWCancel, error) {
	return nil, nil
}
