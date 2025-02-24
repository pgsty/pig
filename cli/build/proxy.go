package build

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	proxyVer            = "5.28.0"
	proxyConfigTemplate = `{
  "log": {"loglevel": "info"},
  "inbounds": [{"port": %s,"listen": "%s","protocol": "http","tag": "http-in","settings": {}}],
  "outbounds": [{
      "tag": "vmess-out", "protocol": "vmess", "settings": {
        "vnext": [{"address": "%s", "port": %s,"users": [{"id": "%s","alterId": 0}]}]
      },
      "streamSettings": {"network": "tcp","security": "none","tcpSettings": {"header": {"type": "none"}}}
    }]
}`
	proxyEnvTemplate = `
alias po="export HTTP_PROXY=%s && export HTTPS_PROXY=%s && export ALL_PROXY=%s"
alias px="unset HTTP_PROXY && unset HTTPS_PROXY && unset ALL_PROXY && unset NO_PROXY"
alias pck="curl -I --connect-timeout 10 www.google.com"
`
)

// SetupProxy sets up proxy with the given remote and local configurations
func SetupProxy(remote string, local string) error {
	if local == "" { // Default local IP and port
		local = "127.0.0.1:12345"
	}
	logrus.Infof("setting up proxy from local %s to remote %s", local, remote)

	// 1. Parse target address (user@host:port)
	parts := strings.Split(remote, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid target format, expected user@host:port, got %s", remote)
	}
	userID := parts[0]
	hostPort := strings.Split(parts[1], ":")
	if len(hostPort) != 2 {
		return fmt.Errorf("invalid host:port format, expected host:port, got %s", parts[1])
	}
	host := hostPort[0]
	port := hostPort[1]

	localHost := strings.Split(local, ":")[0]
	localPort := strings.Split(local, ":")[1]

	// 2. Install or make sure proxy packages are installed
	if err := InstallProxy(); err != nil {
		return fmt.Errorf("failed to install proxy: %v", err)
	}

	// 3. Write proxy config
	proxyConfig := fmt.Sprintf(proxyConfigTemplate, localPort, localHost, host, port, userID)
	if err := utils.PutFile("/etc/v2ray.json", []byte(proxyConfig)); err != nil {
		logrus.Debugf("fail to write /etc/v2ray.json content: %s", proxyConfig)
		return fmt.Errorf("failed to write /etc/v2ray.json: %v", err)
	} else {
		logrus.Info("write proxy config to /etc")
		logrus.Debugf("/etc/v2ray.json content: %s", proxyConfig)
	}

	// 4. Write proxy environment
	proxyURL := "http://" + local
	proxyEnv := fmt.Sprintf(proxyEnvTemplate, proxyURL, proxyURL, proxyURL)
	if err := utils.PutFile("/etc/profile.d/proxy.sh", []byte(proxyEnv)); err != nil {
		return fmt.Errorf("failed to write /etc/profile.d/proxy.sh: %v", err)
	}

	// 5. Restart proxy service
	if err := utils.SudoCommand([]string{"systemctl", "restart", "v2ray"}); err != nil {
		return fmt.Errorf("failed to restart proxy service: %v", err)
	} else {
		logrus.Info("proxy service started")
	}

	// 6. Test proxy connectivity
	if err := testProxy(local); err != nil {
		logrus.Errorf("proxy is not working: %v", err)
		logrus.Warn("check proxy status with:")
		logrus.Warn("$ systemctl status v2ray")
		logrus.Warn("$ cat /etc/v2ray.json")
		return nil
	} else {
		logrus.Info("proxy is working, turn on/off proxy with:")
		logrus.Infof("Proxy On    : \t$ po")
		logrus.Infof("Proxy Off   : \t$ px")
		logrus.Infof("Proxy Check : \t$ pck")
		logrus.Infof("Proxy Alias : \t$ . /etc/profile.d/proxy.sh")
	}
	return nil
}

// InstallProxy will install build proxy via http
func InstallProxy() error {
	if _, err := os.Stat("/usr/local/bin/v2ray"); err == nil {
		logrus.Info("proxy binary is already installed")
		return nil
	}

	var filename, packageURL, downloadTo string
	switch config.OSType {
	case config.DistroEL:
		osarch := config.OSArch
		switch osarch {
		case "amd64", "x86_64":
			osarch = "x86_64"
		case "arm64", "aarch64":
			osarch = "aarch64"
		default:
			return fmt.Errorf("unsupported arch: %s on %s %s", osarch, config.OSType, config.OSCode)
		}
		logrus.Debugf("osarch=%s, proxyVer=%s", osarch, proxyVer)
		filename = fmt.Sprintf("vray-%s-1.%s.rpm", proxyVer, osarch)
		packageURL = fmt.Sprintf("%s/pkg/ray/%s", config.RepoPigstyCC, filename)
	case config.DistroDEB:
		logrus.Debugf("osarch=%s, proxy=%s", config.OSArch, proxyVer)
		filename = fmt.Sprintf("vray_%s-1_%s.deb", proxyVer, config.OSArch)
		packageURL = fmt.Sprintf("%s/pkg/ray/%s", config.RepoPigstyCC, filename)
	case config.DistroMAC:
		return fmt.Errorf("macos is not supported yet")
	}
	downloadTo = fmt.Sprintf("/tmp/%s", filename)

	logrus.Debugf("wipe destination file %s", downloadTo)
	if err := utils.DelFile(downloadTo); err != nil {
		return fmt.Errorf("failed to wipe destination file: %v", err)
	}
	logrus.Debugf("downloading proxy %s package from %s to %s", config.OSType, packageURL, downloadTo)
	if err := utils.DownloadFile(packageURL, downloadTo); err != nil {
		return fmt.Errorf("failed to download package: %v", err)
	}

	// run sudo shell command to remove current package and install the new one
	logrus.Debugf("reinstall proxy via package manager")
	switch config.OSType {
	case config.DistroEL:
		_ = utils.QuietSudoCommand([]string{"yum", "remove", "-q", "-y", "vray"})
		return utils.SudoCommand([]string{"rpm", "-i", downloadTo})
	case config.DistroDEB:
		_ = utils.QuietSudoCommand([]string{"apt", "remove", "-qq", "-y", "vray"})
		return utils.SudoCommand([]string{"dpkg", "-i", downloadTo})
	}
	return nil
}

// testProxy tests if the proxy is working by trying to access google.com
func testProxy(local string) error {
	// Set proxy URL based on local configuration
	proxyURLStr := "http://" + local
	// Parse proxy URL
	proxyURL, err := url.Parse(proxyURLStr)
	if err != nil {
		logrus.Warnf("invalid proxy url: %v", err)
		return fmt.Errorf("invalid proxy url: %v", err)
	}

	logrus.Info("testing proxy connectivity...")
	for i := 0; i < 15; i++ {
		conn, err := net.DialTimeout("tcp", local, time.Second)
		if err == nil {
			conn.Close()
			logrus.Info("proxy is ready")
			break
		} else {
			logrus.Warnf("proxy is not ready yet, retrying... %d", i)
		}
		time.Sleep(time.Second)
	}

	// Create HTTP client with proxy
	proxyClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 10 * time.Second,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://www.google.com", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Send request
	resp, err := proxyClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to google.com: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	return nil
}
