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

// NetworkCondition probes repository endpoints and returns the fastest responding one.
func NetworkCondition() string {
	return NetworkConditionWithTimeout(DefaultTimeout)
}

// NetworkConditionWithTimeout probes repository endpoints with a specified timeout.
func NetworkConditionWithTimeout(timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	type result struct {
		source string
		data   []VersionInfo
	}
	resultChan := make(chan result, 3)

	probeEndpoint := func(ctx context.Context, baseURL, tag string) {
		url := baseURL + "/src/checksums"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			if Details {
				fmt.Printf("%-10s request error\n", tag)
			}
			resultChan <- result{"", nil}
			return
		}

		client := &http.Client{Timeout: timeout}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			if Details {
				fmt.Printf("%-10s unreachable\n", tag)
			}
			resultChan <- result{"", nil}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			versions, err := ParseChecksums(resp.Body, tag)
			if err != nil {
				if Details {
					fmt.Printf("%-10s parse error: %v\n", tag, err)
				}
				resultChan <- result{"", nil}
				return
			}
			if Details {
				fmt.Printf("%s  ping ok: %d ms\n", tag, time.Since(start).Milliseconds())
			}
			resultChan <- result{tag, versions}
			return
		}
		resultChan <- result{"", nil}
	}

	probeGoogle := func(ctx context.Context) {
		tag := "google.com"
		url := "https://www.google.com"
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
		if err != nil {
			if Details {
				fmt.Printf("%-10s request error\n", tag)
			}
			resultChan <- result{"google", nil}
			return
		}
		client := &http.Client{Timeout: timeout}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			if Details {
				fmt.Printf("%-10s request error\n", tag)
			}
			resultChan <- result{"google", nil}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if Details {
				fmt.Printf("%-10s ping ok: %d ms\n", tag, time.Since(start).Milliseconds())
			}
			resultChan <- result{"google_ok", nil}
		} else {
			if Details {
				fmt.Printf("%-10s ping fail: %d ms\n", tag, time.Since(start).Milliseconds())
			}
			resultChan <- result{"google_fail", nil}
		}
	}

	go probeGoogle(ctx)
	go probeEndpoint(ctx, config.RepoPigstyIO, ViaIO)
	go probeEndpoint(ctx, config.RepoPigstyCC, ViaCC)

	var ioData, ccData []VersionInfo
	ioSuccess := false
	ccSuccess := false
	googleSuccess := false
	receivedCount := 0

	for receivedCount < 3 {
		select {
		case res := <-resultChan:
			receivedCount++
			switch res.source {
			case ViaIO:
				if len(res.data) > 0 {
					ioSuccess = true
					ioData = res.data
				}
			case ViaCC:
				if len(res.data) > 0 {
					ccSuccess = true
					ccData = res.data
				}
			case "google_ok":
				googleSuccess = true
			}
		case <-ctx.Done():
			goto DONE
		}
	}

DONE:
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
