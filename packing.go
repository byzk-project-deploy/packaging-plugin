package packaging_plugin

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	rpcinterfaces "github.com/byzk-project-deploy/base-interface"
	"github.com/go-base-lib/coderutils"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

var (
	currentOs   rpcinterfaces.OsOrArch
	currentArch rpcinterfaces.OsOrArch
)

func CurrentOs() rpcinterfaces.OsOrArch {
	return currentOs
}

func CurrentArch() rpcinterfaces.OsOrArch {
	return currentArch
}

func initOsAndArch() error {
	switch runtime.GOOS {
	case "linux":
		currentOs = rpcinterfaces.OsLinux
	case "darwin":
		currentOs = rpcinterfaces.OsDarwin
	default:
		return fmt.Errorf("未被支持的系统")
	}

	switch runtime.GOARCH {
	case "amd64":
		currentArch = rpcinterfaces.ArchAmd64
	case "arm":
		currentArch = rpcinterfaces.ArchArm64
	case "arm64":
		currentArch = rpcinterfaces.ArchArm64
	case "mips64le":
		currentArch = rpcinterfaces.ArchMips64le
	default:
		return fmt.Errorf("未被支持的架构")
	}
	return nil
}

// Packing 打包插件通过路径
func Packing(src, target string) error {

	_ = os.RemoveAll(target)
	targetDir := filepath.Dir(target)
	_ = os.MkdirAll(targetDir, 0755)

	targetF, err := os.OpenFile(target, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("创建目标文件[%s]失败: %s", target, err.Error())
	}
	defer targetF.Close()

	if err = PackingToWriteStream(src, targetF); err != nil {
		_ = targetF.Close()
		_ = os.RemoveAll(target)
		return err
	}
	return nil
}

// PackingToWriteStream 打包到输出路径
func PackingToWriteStream(src string, target io.ReadWriteSeeker) error {
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

	md5Sum, err := coderutils.HashByReader(md5.New(), target)
	if err != nil {
		return fmt.Errorf("获取插件MD5摘要失败")
	}

	if _, err = target.Seek(0, 0); err != nil {
		return fmt.Errorf("移动插件文件流指针失败: %s", err.Error())
	}

	sha1Sum, err := coderutils.HashByReader(sha1.New(), target)
	if err != nil {
		return fmt.Errorf("获取插件SHA1摘要失败")
	}

	if _, err = target.Seek(-(int64(infoBytesLen) + 4), 2); err != nil {
		return fmt.Errorf("移动插件文件流指针失败: %s", err.Error())
	}

	if _, err = target.Write(md5Sum); err != nil {
		return fmt.Errorf("目标文件写出摘要信息失败: %s", err.Error())
	}

	if _, err = target.Write(sha1Sum); err != nil {
		return fmt.Errorf("目标文件写出摘要信息失败: %s", err.Error())
	}

	if _, err = target.Write(marshal); err != nil {
		return fmt.Errorf("向目标文件写入插件信息失败: %s", err.Error())
	}

	if _, err = target.Write(infoLenBytes); err != nil {
		return fmt.Errorf("向目标写出数据长度失败： %s", err.Error())
	}

	return nil
}

func Unpacking(packingPluginFile, targetPath string) (*rpcinterfaces.PluginInfo, error) {
	f, err := os.OpenFile(packingPluginFile, os.O_RDONLY, 0655)
	if err != nil {
		return nil, fmt.Errorf("打开包装的插件文件失败: %s", err.Error())
	}
	defer f.Close()

	_ = os.RemoveAll(targetPath)
	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
	targetF, err := os.OpenFile(targetPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return nil, fmt.Errorf("打开目标文件失败: %s", err.Error())
	}
	defer targetF.Close()

	if info, err := UnpackingByStream(f, targetF); err != nil {
		_ = targetF.Close()
		_ = os.RemoveAll(targetPath)
		return nil, err
	} else {
		return info, nil
	}
}

type wrapperMultipleReader struct {
	rList        []io.ReadSeeker
	currentIndex int
	total        int
}

func (w *wrapperMultipleReader) Reset() error {
	for i := 0; i < w.total; i++ {
		if _, err := w.rList[i].Seek(0, 0); err != nil {
			return err
		}
	}
	w.currentIndex = 0
	return nil
}

func (w *wrapperMultipleReader) Read(p []byte) (n int, err error) {
Start:
	n, err = w.rList[w.currentIndex].Read(p)
	if err == io.EOF {
		w.currentIndex += 1
		if w.currentIndex < w.total {
			goto Start
		}
	}
	return n, err
}

func newWrapperMultipleReader(r ...io.ReadSeeker) *wrapperMultipleReader {
	_r := &wrapperMultipleReader{
		rList:        r,
		total:        len(r),
		currentIndex: 0,
	}
	_ = _r.Reset()

	return _r
}

func UnpackingByStream(packingPluginFile io.ReadSeekCloser, target io.ReadWriteSeeker) (*rpcinterfaces.PluginInfo, error) {

	lenBit := int64(4)

	if _, err := packingPluginFile.Seek(-lenBit, 2); err != nil {
		return nil, fmt.Errorf("移动文件指针失败: %s", err.Error())
	}

	infoLenBytes, err := readBytesByLen(packingPluginFile, 4)
	if err != nil {
		return nil, fmt.Errorf("读取插件包的数据长度失败: %s", err.Error())
	}

	infoLen, err := BytesToInt[int32](infoLenBytes)
	if err != nil {
		return nil, fmt.Errorf("转换数据长度失败: %s", err.Error())
	}

	infoEndOffset := lenBit + int64(infoLen)

	if _, err = packingPluginFile.Seek(-infoEndOffset, 2); err != nil {
		return nil, fmt.Errorf("移动插件包指针失败: %s", err.Error())
	}

	infoBytes, err := readBytesByLen(packingPluginFile, int(infoLen))
	if err != nil {
		return nil, fmt.Errorf("读取包内插件信息失败: %s", err.Error())
	}

	var pluginInfo *rpcinterfaces.PluginInfo
	if err = json.Unmarshal(infoBytes, &pluginInfo); err != nil {
		return nil, fmt.Errorf("反序列化插件信息失败: %s", err.Error())
	}

	hashEndOffset := infoEndOffset + md5.Size + sha1.Size
	dataStartPos, err := packingPluginFile.Seek(-hashEndOffset, 2)
	if err != nil {
		return nil, fmt.Errorf("移动插件包文件指针失败: %s", err.Error())
	}

	md5Sum, err := readBytesByLen(packingPluginFile, md5.Size)
	if err != nil {
		return nil, fmt.Errorf("读取MD5摘要失败")
	}

	sha1Sum, err := readBytesByLen(packingPluginFile, sha1.Size)
	if err != nil {
		return nil, fmt.Errorf("读取SHA1摘要失败")
	}

	if _, err = packingPluginFile.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("移动插件包文件指针失败: %s", err.Error())
	}

	if _, err = io.CopyN(target, packingPluginFile, dataStartPos); err != nil {
		return nil, fmt.Errorf("写出插件数据失败: %s", err.Error())
	}

	multipleReader := newWrapperMultipleReader(target, bytes.NewReader(bytes.Join([][]byte{
		infoBytes,
		infoLenBytes,
	}, nil)))
	targetMd5Sum, err := coderutils.HashByReader(md5.New(), multipleReader)
	if err != nil {
		return nil, fmt.Errorf("获取目标文件的MD5摘要失败")
	}

	if !bytes.Equal(targetMd5Sum, md5Sum) {
		return nil, fmt.Errorf("插件包完整性校验失败")
	}

	if err = multipleReader.Reset(); err != nil {
		return nil, fmt.Errorf("移动目标文件指针失败: %s", err.Error())
	}

	targetSha1Sum, err := coderutils.HashByReader(sha1.New(), multipleReader)
	if err != nil {
		return nil, fmt.Errorf("获取目标文件的SHA1摘要失败")
	}

	if !bytes.Equal(targetSha1Sum, sha1Sum) {
		return nil, fmt.Errorf("插件包完整性校验失败")
	}

	if pluginInfo.NotAllowOsAndArch != nil {
		for i := range pluginInfo.NotAllowOsAndArch {
			if pluginInfo.NotAllowOsAndArch[i].Is(currentOs, currentArch) {
				return nil, fmt.Errorf("不支持在当前系统中运行")
			}
		}
	}

	if pluginInfo.AllowOsAndArch != nil {
		for i := range pluginInfo.AllowOsAndArch {
			if pluginInfo.AllowOsAndArch[i].Is(currentOs, currentArch) {
				goto Success
			}
		}
	} else {
		goto Success
	}
	return nil, fmt.Errorf("插件不允许在当前系统中运行")

Success:
	return pluginInfo, nil

}

func readBytesByLen(r io.Reader, l int) (res []byte, err error) {
	res = make([]byte, l)
	buf := make([]byte, 1)

	for i := 0; i < l; i++ {
		if n, err := r.Read(buf); err != nil {
			return nil, err
		} else if n != 1 {
			return nil, fmt.Errorf("数据读取失败")
		}
		res[i] = buf[0]
	}
	return
}

// getPluginInfoByPath 通过插件路径获取插件信息
func getPluginInfoByPath(p string) (*rpcinterfaces.PluginInfo, error) {
	if _p, err := filepath.Abs(p); err != nil {
		return nil, fmt.Errorf("获取插件文件的绝对路径失败: %s", err.Error())
	} else {
		p = _p
	}

	pluginMap := map[string]plugin.Plugin{
		rpcinterfaces.PluginNameInfo: &rpcinterfaces.PluginInfoImpl{},
	}

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: *rpcinterfaces.DefaultHandshakeConfig,
		Logger:          hclog.NewNullLogger(),
		Stderr:          ioutil.Discard,
		Plugins:         pluginMap,
		Cmd:             exec.Command(p),
	})
	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		return nil, err
	}

	raw, err := rpcClient.Dispense(rpcinterfaces.PluginNameInfo)
	if err != nil {
		return nil, err
	}

	applicationPlugin := raw.(rpcinterfaces.PluginInfoInterface)
	return applicationPlugin.Info()
}
