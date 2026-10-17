package build

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"pig/cli/ext"
	"pig/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	proxyConfigPath = "/etc/v2ray.json"

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
	userID, host, port, err := parseRemoteTarget(remote)
	if err != nil {
		return err
	}
	localHost, localPort, local, err := parseLocalListen(local)
	if err != nil {
		return err
	}
	logrus.Infof("setting up proxy from local %s to remote %s", local, net.JoinHostPort(host, port))

	// 2. Install or make sure proxy packages are installed
	if err := installLegacyProxy(); err != nil {
		return fmt.Errorf("failed to install proxy: %v", err)
	}

	// 3. Write proxy config
	proxyConfig := fmt.Sprintf(proxyConfigTemplate, localPort, localHost, host, port, userID)
	// The packaged unit runs as v2ray:v2ray on both RPM and DEB systems.
	if err := utils.PutFileMode(proxyConfigPath, []byte(proxyConfig), 0640); err != nil {
		return fmt.Errorf("failed to write %s: %v", proxyConfigPath, err)
	} else if err := utils.SudoCommand([]string{"chown", "root:v2ray", proxyConfigPath}); err != nil {
		return fmt.Errorf("failed to grant the v2ray service access to %s: %v", proxyConfigPath, err)
	} else {
		logrus.Info("write proxy config to /etc")
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
		logrus.Warn("$ journalctl -u v2ray -n 50")
		return fmt.Errorf("proxy connectivity check failed: %w", err)
	} else {
		logrus.Info("proxy is working, turn on/off proxy with:")
		logrus.Infof("Proxy On    : \t$ po")
		logrus.Infof("Proxy Off   : \t$ px")
		logrus.Infof("Proxy Check : \t$ pck")
		logrus.Infof("Proxy Alias : \t$ . /etc/profile.d/proxy.sh")
	}
	return nil
}

func parseRemoteTarget(remote string) (userID string, host string, port string, err error) {
	parts := strings.SplitN(remote, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", fmt.Errorf("invalid target format, expected user@host:port")
	}
	host, port, err = net.SplitHostPort(parts[1])
	if err != nil {
		return "", "", "", fmt.Errorf("invalid host:port format, expected host:port")
	}
	if host == "" {
		return "", "", "", fmt.Errorf("invalid host:port format, host is empty")
	}
	if err := validatePort(port); err != nil {
		return "", "", "", err
	}
	return parts[0], host, port, nil
}

func parseLocalListen(local string) (host string, port string, normalized string, err error) {
	if local == "" {
		local = "127.0.0.1:12345"
	}

	host, port, err = net.SplitHostPort(local)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid local listen address, expected host:port")
	}
	if err := validatePort(port); err != nil {
		return "", "", "", err
	}

	switch host {
	case "":
		host = "127.0.0.1"
	case "*":
		host = "0.0.0.0"
	}

	return host, port, net.JoinHostPort(host, port), nil
}

func validatePort(port string) error {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("invalid port (must be in range 1-65535)")
	}
	return nil
}

// InstallProxy installs Xray for the new explicit client/server workflow.
func InstallProxy() error {
	return installXray()
}

func installLegacyProxy() error {
	if _, err := exec.LookPath("v2ray"); err == nil {
		logrus.Info("proxy binary is already installed")
		return nil
	}

	manager := ext.PackageManagerCmd()
	command, err := proxyInstallCommand(manager)
	if err != nil {
		return err
	}
	logrus.Infof("install proxy from configured package repositories with %s", manager)
	return utils.SudoCommand(command)
}

func proxyInstallCommand(manager string) ([]string, error) {
	switch manager {
	case "dnf", "yum", "apt-get":
		return []string{manager, "install", "-y", "vray"}, nil
	default:
		return nil, fmt.Errorf("proxy installation requires dnf, yum, or apt-get; configure the Pigsty repository first")
	}
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
