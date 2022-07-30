package packaging_plugin

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	rpcinterfaces "github.com/byzk-project-deploy/base-interface"
	"github.com/go-base-lib/coderutils"
	"github.com/hashicorp/go-plugin"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Packing 打包插件通过路径
func Packing(src, target string) error {

	_ = os.RemoveAll(target)
	targetDir := filepath.Dir(target)
	_ = os.MkdirAll(targetDir, 0755)

	targetF, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE, 0655)
	if err != nil {
		return fmt.Errorf("创建目标文件[%s]失败: %s", target, err.Error())
	}
	defer targetF.Close()

	return PackingToWriteStream(src, targetF)
}

// PackingToWriteStream 打包到输出路径
func PackingToWriteStream(src string, target io.WriteSeeker) error {
	pluginInfo, err := getPluginInfoByPath(src)
	if err != nil {
		return err
	}

	marshal, err := json.Marshal(pluginInfo)
	if err != nil {
		return fmt.Errorf("序列化插件信息失败: %s", err.Error())
	}

	infoBytesLen := int32(len(marshal))
	infoLenBytes, err := IntToBytes(infoBytesLen)
	if err != nil {
		return fmt.Errorf("转换数据长度失败: %s", err.Error())
	}

	f, err := os.OpenFile(src, os.O_RDONLY, 0655)
	if err != nil {
		return fmt.Errorf("打开插件文件[%s]失败: %s", src, err.Error())
	}
	defer f.Close()

	_, err = io.Copy(target, f)
	if err != nil {
		return fmt.Errorf("向目标写出插件数据失败: %s", err.Error())
	}

	if _, err = target.Write(marshal); err != nil {
		return fmt.Errorf("向目标地址写出插件信息失败: %s", err.Error())
	}

	if _, err = target.Write(infoLenBytes); err != nil {
		return fmt.Errorf("向目标地址写出插件信息长度失败: %s", err.Error())
	}

	if _, err = target.Seek(0, 0); err != nil {
		return fmt.Errorf("移动插件文件流指针失败: %s", err.Error())
	}

	md5Sum, err := coderutils.HashByReader(md5.New(), f)
	if err != nil {
		return fmt.Errorf("获取插件MD5摘要失败")
	}

	if _, err = target.Seek(0, 0); err != nil {
		return fmt.Errorf("移动插件文件流指针失败: %s", err.Error())
	}

	sha1Sum, err := coderutils.HashByReader(sha1.New(), f)
	if err != nil {
		return fmt.Errorf("获取插件SHA1摘要失败")
	}

	if _, err = target.Seek(2, -(int(infoBytesLen) + 4)); err != nil {
		return fmt.Errorf("移动插件文件流指针失败: %s", err.Error())
	}

	if _, err = target.Write(md5Sum); err != nil {
		return fmt.Errorf("目标文件写出摘要信息失败: %s", err.Error())
	}

	if _, err = target.Write(sha1Sum); err != nil {
		return fmt.Errorf("目标文件写出摘要信息失败: %s", err.Error())
	}
	return nil
}

// getPluginInfoByPath 通过插件路径获取插件信息
func getPluginInfoByPath(p string) (*rpcinterfaces.PluginInfo, error) {
	pluginMap := map[string]plugin.Plugin{
		rpcinterfaces.PluginNameInfo: &rpcinterfaces.PluginInfoImpl{},
	}

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "BASIC_PLUGIN",
			MagicCookieValue: "hello",
		},

		Plugins: pluginMap,
		Cmd:     exec.Command(p),
	})
	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		panic(err)
	}

	raw, err := rpcClient.Dispense(rpcinterfaces.PluginNameInfo)
	if err != nil {
		return nil, err
	}

	applicationPlugin := raw.(rpcinterfaces.PluginInfoInterface)
	return applicationPlugin.Info()
}
