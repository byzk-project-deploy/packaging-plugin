package packaging_plugin

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPackingAndUnpacking(t *testing.T) {
	a := assert.New(t)

	err := Packing("test", "test_target")

}
