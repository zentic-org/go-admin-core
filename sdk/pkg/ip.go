package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

// GetLocation 获取外网ip地址
func GetLocation(ip, key string) string {
	if ip == "127.0.0.1" || ip == "localhost" {
		return "内部IP"
	}
	url := "https://restapi.amap.com/v5/ip?ip=" + ip + "&type=4&key=" + key
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Failed to get response from restapi.amap.com:", err)
		return "未知位置"
	}
	defer resp.Body.Close()

	s, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Failed to read response body:", err)
		return "未知位置"
	}

	var m map[string]string
	err = json.Unmarshal(s, &m)
	if err != nil {
		fmt.Println("Failed to unmarshal response:", err)
		return "未知位置"
	}

	// Construct the location string from the response map
	location := fmt.Sprintf("%s-%s-%s-%s-%s", m["country"], m["province"], m["city"], m["district"], m["isp"])
	return location
}

// GetLocalHost 获取局域网ip地址
func GetLocalHost() string {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("Failed to get network interfaces:", err)
		return ""
	}

	for _, netInterface := range netInterfaces {
		if (netInterface.Flags & net.FlagUp) != 0 {
			addrs, err := netInterface.Addrs()
			if err != nil {
				fmt.Println("Failed to get addresses for interface:", err)
				continue
			}
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						return ipnet.IP.String()
					}
				}
			}
		}
	}
	return ""
}
