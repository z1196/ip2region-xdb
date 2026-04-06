package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxmind/mmdbwriter"
)

const (
	dataDir   = "data"
	outputMMDB = "GeoCN.mmdb"

	ipv4Src = "ipv4_source.txt"
	ipv6Src = "ipv6_source.txt"
)

type Record struct {
	Country string `maxminddb:"country"`
	Region  string `maxminddb:"region"`
	City    string `maxminddb:"city"`
	ISP     string `maxminddb:"isp"`
	ASN     string `maxminddb:"asn"`
}

func parseLine(line string) (string, string, Record, bool) {
	parts := strings.Split(strings.TrimSpace(line), "|")
	if len(parts) < 7 {
		return "", "", Record{}, false
	}

	return parts[0], parts[1], Record{
		Country: parts[2],
		Region:  parts[3],
		City:    parts[4],
		ISP:     parts[5],
		ASN:     parts[6],
	}, true
}

func cidrRange(startIP, endIP net.IP) []net.IPNet {
	var cidrs []net.IPNet
	for ip := startIP; compareIP(ip, endIP) <= 0; {
		maxSize := maxCIDR(ip, endIP)
		_, network, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), maxSize))
		cidrs = append(cidrs, *network)
		ip = nextIP(network)
	}
	return cidrs
}

func compareIP(a, b net.IP) int {
	return bytesCompare(a.To16(), b.To16())
}

func bytesCompare(a, b []byte) int {
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func maxCIDR(startIP, endIP net.IP) int {
	maxMask := 128
	for prefix := 128; prefix >= 0; prefix-- {
		_, network, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", startIP.String(), prefix))
		if compareIP(lastIP(network), endIP) <= 0 {
			maxMask = prefix
			break
		}
	}
	return maxMask
}

func lastIP(n *net.IPNet) net.IP {
	ip := n.IP.To16()
	mask := n.Mask
	broadcast := make(net.IP, len(ip))
	for i := range ip {
		broadcast[i] = ip[i] | ^mask[i]
	}
	return broadcast
}

func nextIP(n *net.IPNet) net.IP {
	ip := lastIP(n)
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
	return ip
}

func processFile(writer *mmdbwriter.Tree, filePath string) {
	f, err := os.Open(filePath)
	if err != nil {
		log.Printf("skip missing file: %s", filePath)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		start, end, record, ok := parseLine(scanner.Text())
		if !ok {
			continue
		}

		startIP := net.ParseIP(start)
		endIP := net.ParseIP(end)
		if startIP == nil || endIP == nil {
			continue
		}

		cidrs := cidrRange(startIP, endIP)
		for _, cidr := range cidrs {
			err := writer.Insert(cidr, record)
			if err != nil {
				log.Printf("insert error: %v", err)
			}
		}
	}
}

func main() {
	outputPath := filepath.Join(dataDir, outputMMDB)
	fmt.Println("Building MMDB:", outputPath)

	writer, err := mmdbwriter.New(
		mmdbwriter.Options{
			DatabaseType: "GeoCN",
			Languages:    []string{"zh-CN"},
			Description:  map[string]string{"zh-CN": "GeoCN mmdb"},
		},
	)
	if err != nil {
		log.Fatalf("writer init error: %v", err)
	}

	processFile(writer, filepath.Join(dataDir, ipv4Src))
	processFile(writer, filepath.Join(dataDir, ipv6Src))

	err = writer.WriteToFile(outputPath)
	if err != nil {
		log.Fatalf("write mmdb error: %v", err)
	}

	fmt.Println("MMDB build completed:", outputPath)
}
