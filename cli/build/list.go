package build

import (
	"github.com/sirupsen/logrus"
)

// Extension 扩展信息
type Extension struct {
	Name        string
	Version     string
	Status      string
	Description string
}

// ListExtensions 列出扩展
func ListExtensions(extensions []string) error {
	if len(extensions) == 0 {
		return listAllExtensions()
	}

	for _, ext := range extensions {
		logrus.Infof("listing extension: %s", ext)
		if err := listExtension(ext); err != nil {
			return err
		}
	}
	return nil
}

// listAllExtensions 列出所有扩展
func listAllExtensions() error {
	// TODO: 实现列出所有扩展的逻辑
	// 1. 扫描扩展目录
	// 2. 收集扩展信息
	// 3. 格式化输出
	return nil
}

// listExtension 列出特定扩展
func listExtension(extension string) error {
	// TODO: 实现列出特定扩展的逻辑
	// 1. 获取扩展信息
	// 2. 格式化输出
	return nil
}

// getExtensionInfo 获取扩展信息
func getExtensionInfo(extension string) (*Extension, error) {
	// TODO: 实现获取扩展信息的逻辑
	return nil, nil
}

// formatExtensionInfo 格式化扩展信息
func formatExtensionInfo(ext *Extension) string {
	// TODO: 实现扩展信息格式化逻辑
	return ""
}
