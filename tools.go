package packaging_plugin

import (
	"bytes"
	"encoding/binary"
)

type IntType interface {
	uint | uint8 | uint16 | uint32 | uint64 | int | int8 | int16 | int32 | int64
}

// IntToBytes int转bytes使用长整型, 8个字节
func IntToBytes[T IntType](n T) ([]byte, error) {

	buf := &bytes.Buffer{}

	if err := binary.Write(buf, binary.BigEndian, n); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func BytesToInt[T IntType](b []byte) (T, error) {
	buf := bytes.NewBuffer(b)

	var r T

	if err := binary.Read(buf, binary.BigEndian, &r); err != nil {
		return 0, err
	}
	return r, nil
}

func ByteToBool(b byte) bool {
	return b > 0
}

func BoolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
