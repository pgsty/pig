package boot

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	region   string
	pkgPath  string
	keepRepo bool
)

func Bootstrap() error {
	pigstyHome := os.Getenv("PIGSTY_HOME")
	if pigstyHome == "" {
		return fmt.Errorf("PIGSTY_HOME environment variable is not set")
	}

	bootstrapPath := filepath.Join(pigstyHome, "bootstrap")

	cmdArgs := []string{"shell"}
	if region != "" {
		cmdArgs = append(cmdArgs, "-r", region)
	}
	if pkgPath != "" {
		cmdArgs = append(cmdArgs, "-p", pkgPath)
	}
	if keepRepo {
		cmdArgs = append(cmdArgs, "-k")
	}

	// 创建命令
	command := exec.Command(bootstrapPath, cmdArgs...)
	command.Dir = pigstyHome

	// 捕获输出
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	// 执行命令
	if err := command.Run(); err != nil {
		return fmt.Errorf("bootstrap execution failed: %v\nStderr: %s", err, stderr.String())
	}

	// 打印输出
	fmt.Print(stdout.String())
	return nil
}
