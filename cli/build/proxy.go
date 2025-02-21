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

const proxyConfigTemplate = `{
  "log": {"loglevel": "info"},
  "inbounds": [{"port": %s,"listen": "%s","protocol": "http","tag": "http-in","settings": {}}],
  "outbounds": [{
      "tag": "vmess-out", "protocol": "vmess", "settings": {
        "vnext": [{"address": "%s", "port": %s,"users": [{"id": "%s","alterId": 0}]}]
      },
      "streamSettings": {"network": "tcp","security": "none","tcpSettings": {"header": {"type": "none"}}}
    }]
}`

// proxy environment template
const proxyEnvTemplate = `
alias po="export HTTP_PROXY=%s && export HTTPS_PROXY=%s && export ALL_PROXY=%s"
alias px="unset HTTP_PROXY && unset HTTPS_PROXY && unset ALL_PROXY && unset NO_PROXY"
alias pck="curl -I --connect-timeout 10 www.google.com"
`

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
	if err := installProxy(); err != nil {
		return fmt.Errorf("failed to install proxy: %v", err)
	}

	// 3. Write proxy config
	proxyConfig := fmt.Sprintf(proxyConfigTemplate, localPort, localHost, host, port, userID)
	if err := utils.PutFile("/etc/v2ray.json", []byte(proxyConfig)); err != nil {
		logrus.Debugf("fail to write /etc/v2ray.json content: %s", proxyConfig)
		return fmt.Errorf("failed to write /etc/v2ray.json: %v", err)
	} else {
		logrus.Info("write /etc/v2ray.json config")
		logrus.Debugf("write /etc/v2ray.json content: %s", proxyConfig)
	}

	// 4. Write proxy environment
	proxyURL := "http://" + local
	proxyEnv := fmt.Sprintf(proxyEnvTemplate, proxyURL, proxyURL, proxyURL)
	if err := utils.PutFile("/etc/profile.d/proxy.sh", []byte(proxyEnv)); err != nil {
		return fmt.Errorf("failed to write /etc/profile.d/proxy.sh: %v", err)
	}

	// 5. Restart proxy service
	if err := utils.SudoCommand([]string{"systemctl", "restart", "v2ray"}); err != nil {
		return fmt.Errorf("failed to restart v2ray service: %v", err)
	} else {
		logrus.Info("start v2ray service")
	}

	// 6. Test proxy connectivity
	if err := testProxy(local); err != nil {
		logrus.Errorf("proxy is not working: %v", err)
		logrus.Warn("check proxy status with:")
		logrus.Warn("$ systemctl status v2ray")
		logrus.Warn("$ cat /etc/v2ray.json")
		return nil
	} else {
		logrus.Info("proxy is working")
		logrus.Infof("Proxy On    : \t$ po")
		logrus.Infof("Proxy Off   : \t$ px")
		logrus.Infof("Proxy Check : \t$ pck")
		logrus.Infof("Proxy Alias : \t$ . /etc/profile.d/proxy.sh")
	}
	return nil
}

// installProxy installs the proxy if not already installed
func installProxy() error {
	// Check if proxy is already installed
	if _, err := os.Stat("/usr/local/bin/v2ray"); err == nil {
		logrus.Info("proxy binary is already installed")
		return nil
	}

	switch config.OSType {
	case config.DistroEL:
		logrus.Info("proxy package not found, try install via yum")
		if err := utils.SudoCommand([]string{"sudo", "yum", "install", "-y", "vray"}); err != nil {
			return fmt.Errorf("failed to install proxy via yum: %v", err)
		}
	case config.DistroDEB:
		logrus.Info("proxy package not found, try install via apt")
		if err := utils.SudoCommand([]string{"sudo", "apt", "install", "-y", "vray"}); err != nil {
			return fmt.Errorf("failed to install proxy via apt: %v", err)
		}

	case config.DistroMAC:
		logrus.Warn("proxy package not found, MacOs is not supported yet")
		return fmt.Errorf("MacOs is not supported yet")
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
