package get

import (
	"context"
	"fmt"
	"net/http"
	"pig/internal/config"
	"strings"
	"time"
)

var DefaultTimeout = 1500 * time.Millisecond

const (
	ViaIO = "pigsty.io"
	ViaCC = "pigsty.cc"
	ViaNA = "nowhere"
)

var (
	Source         = ViaNA
	Region         = "default"
	InternetAccess = false
	LatestVersion  = config.PigstyVersion
	AllVersions    []VersionInfo
	Details        = false
)

// VersionInfo represents version metadata including checksum and download URL
// Used by network probing, checksum parsing, and source package download.
type VersionInfo struct {
	Version     string
	Checksum    string
	DownloadURL string
}

type probeResult struct {
	tag      string
	ok       bool
	versions []VersionInfo
}

func probeChecksumsEndpoint(ctx context.Context, timeout time.Duration, baseURL, tag string, details bool, resultChan chan<- probeResult) {
	url := baseURL + "/src/checksums"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		if details {
			fmt.Printf("%-10s request error\n", tag)
		}
		resultChan <- probeResult{tag: tag}
		return
	}

	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if details {
			fmt.Printf("%-10s unreachable\n", tag)
		}
		resultChan <- probeResult{tag: tag}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		resultChan <- probeResult{tag: tag}
		return
	}

	versions, err := ParseChecksums(resp.Body, tag)
	if err != nil {
		if details {
			fmt.Printf("%-10s parse error: %v\n", tag, err)
		}
		resultChan <- probeResult{tag: tag}
		return
	}

	if details {
		fmt.Printf("%s  ping ok: %d ms\n", tag, time.Since(start).Milliseconds())
	}

	resultChan <- probeResult{
		tag:      tag,
		ok:       len(versions) > 0,
		versions: versions,
	}
}

func probeGoogle(ctx context.Context, timeout time.Duration, details bool, resultChan chan<- probeResult) {
	printTag := "google.com"
	url := "https://www.google.com"

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		if details {
			fmt.Printf("%-10s request error\n", printTag)
		}
		resultChan <- probeResult{tag: "google"}
		return
	}

	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if details {
			fmt.Printf("%-10s request error\n", printTag)
		}
		resultChan <- probeResult{tag: "google"}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		if details {
			fmt.Printf("%-10s ping ok: %d ms\n", printTag, time.Since(start).Milliseconds())
		}
		resultChan <- probeResult{tag: "google", ok: true}
		return
	}

	if details {
		fmt.Printf("%-10s ping fail: %d ms\n", printTag, time.Since(start).Milliseconds())
	}
	resultChan <- probeResult{tag: "google"}
}

// NetworkCondition probes repository endpoints and returns the fastest responding one.
func NetworkCondition() string {
	return NetworkConditionWithTimeout(DefaultTimeout)
}

// NetworkConditionWithTimeout probes repository endpoints with a specified timeout.
func NetworkConditionWithTimeout(timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan probeResult, 3)

	go probeGoogle(ctx, timeout, Details, resultChan)
	go probeChecksumsEndpoint(ctx, timeout, config.RepoPigstyIO, ViaIO, Details, resultChan)
	go probeChecksumsEndpoint(ctx, timeout, config.RepoPigstyCC, ViaCC, Details, resultChan)

	var ioData, ccData []VersionInfo
	ioSuccess := false
	ccSuccess := false
	googleSuccess := false
	receivedCount := 0

LOOP:
	for receivedCount < 3 {
		select {
		case res := <-resultChan:
			receivedCount++
			switch res.tag {
			case ViaIO:
				if res.ok {
					ioSuccess = true
					ioData = res.versions
				}
			case ViaCC:
				if res.ok {
					ccSuccess = true
					ccData = res.versions
				}
			case "google":
				googleSuccess = res.ok
			}
		case <-ctx.Done():
			break LOOP
		}
	}
	if ioSuccess {
		Source = ViaIO
		AllVersions = ioData
	} else if ccSuccess {
		Source = ViaCC
		AllVersions = ccData
	} else {
		Source = ViaNA
		AllVersions = nil
	}

	InternetAccess = ioSuccess || ccSuccess || googleSuccess
	if InternetAccess && !googleSuccess {
		Region = "china"
	}

	for _, v := range AllVersions {
		if !strings.Contains(v.Version, "-") {
			LatestVersion = v.Version
			break
		}
	}

	if Details {
		fmt.Println("Internet Access   : ", InternetAccess)
		fmt.Println("Pigsty Repo       : ", Source)
		fmt.Println("Inferred Region   : ", Region)
		fmt.Println("Latest Pigsty Ver : ", LatestVersion)
	}

	return Source
}

func PrintAllVersions(since string) {
	if since == "" {
		since = "v3.0.0"
	}
	if AllVersions == nil {
		NetworkCondition()
		if AllVersions == nil {
			return
		}
	}
	const format = "%-10s\t%-36s\t%s\n"
	fmt.Printf(format, "VERSION", "CHECKSUM", "DOWNLOAD URL")
	fmt.Println(strings.Repeat("-", 90))
	for _, v := range AllVersions {
		if CompareVersions(v.Version, since) >= 0 {
			fmt.Printf(format, v.Version, v.Checksum, v.DownloadURL)
		}
	}
}
