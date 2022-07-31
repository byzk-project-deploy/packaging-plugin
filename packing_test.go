package packaging_plugin

import (
	"crypto/md5"
	"github.com/go-base-lib/coderutils"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestPackingAndUnpacking(t *testing.T) {
	var (
		srcFilePath = "test"

		targetFilePath    = "test_pack"
		unpackingFilePath = "test_unpack"
	)

	a := assert.New(t)

	defer os.RemoveAll(targetFilePath)
	defer os.RemoveAll(unpackingFilePath)

	if err := Packing(srcFilePath, targetFilePath); !a.NoError(err) {
		return
	}

	info, targetFilePath, err := Unpacking(targetFilePath, unpackingFilePath)
	if !a.NoError(err) {
		return
	}

	srcMd5Sum, err := coderutils.HashByFilePath(md5.New(), srcFilePath)
	if !a.NoError(err) {
		return
	}

	unpackMd5Sum, err := coderutils.HashByFilePath(md5.New(), unpackingFilePath)
	if !a.NoError(err) {
		return
	}

	if !a.Equal(srcMd5Sum, unpackMd5Sum, "解包之后的插件与原插件包hash不一致") {
		return
	}

	unpackPluginInfo, err := getPluginInfoByPath(unpackingFilePath)
	if !a.NoError(err) {
		return
	}

	a.Equal(unpackPluginInfo, info, "解包之后的运行结果与原包不一致")
	a.Equal(targetFilePath, unpackingFilePath)

}
