package tn3270

import (
	"testing"
)

func TestNewModuleInstance(t *testing.T) {
	_ = &RootModule{}
}

func TestExportsContainsTN3270(t *testing.T) {
	mi := &ModuleInstance{}
	exports := mi.Exports()

	if exports.Named == nil {
		t.Fatal("Exports.Named should not be nil")
	}

	if _, ok := exports.Named["TN3270"]; !ok {
		t.Error("Exports should contain 'TN3270' in Named exports")
	}
}
