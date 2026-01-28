// Package tn3270 provides TN3270 terminal emulation support for k6.
package tn3270

import (
	"go.k6.io/k6/js/modules"
)

func init() {
	modules.Register("k6/x/tn3270", new(RootModule))
}

type RootModule struct{}

type ModuleInstance struct {
	vu modules.VU
}

var _ modules.Module = &RootModule{}
var _ modules.Instance = &ModuleInstance{}

func (*RootModule) NewModuleInstance(vu modules.VU) modules.Instance {
	return &ModuleInstance{vu: vu}
}

func (mi *ModuleInstance) Exports() modules.Exports {
	return modules.Exports{
		Named: map[string]interface{}{
			"TN3270": mi.newTN3270,
		},
	}
}

func (mi *ModuleInstance) newTN3270() *Client {
	return NewClient(mi.vu)
}
