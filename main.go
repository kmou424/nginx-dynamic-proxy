package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	configPath string
	proxyPort  int
	proxyHost  string
	localPort  int
	protocol   string
	refresh    int
)

func parseIntEnv(envName string) (ret int, exist bool) {
	retS, exist := os.LookupEnv(envName)
	if exist {
		retI, err := strconv.Atoi(retS)
		if err == nil {
			ret = retI
			return
		}
	}
	ret = 0
	return
}

func parseStringEnv(envName string) (ret string, exist bool) {
	ret, exist = os.LookupEnv(envName)
	if exist {
		return
	}
	ret = ""
	return
}

func main() {
	flag.StringVar(&configPath, "config", "stream.conf", "config file path")
	flag.IntVar(&proxyPort, "proxy_port", 80, "proxy port")
	flag.StringVar(&proxyHost, "proxy_host", "www.baidu.com", "proxy address")
	flag.IntVar(&localPort, "local_port", 7890, "local port")
	flag.StringVar(&protocol, "protocol", "udp", "proxy protocol")
	flag.IntVar(&refresh, "refresh_interval", 5, "refresh interval (minutes)")
	flag.Parse()

	mConfigPath, exist := parseStringEnv("CONFIG_PATH")
	if exist {
		configPath = mConfigPath
	}
	mProxyPort, exist := parseIntEnv("PROXY_PORT")
	if exist {
		proxyPort = mProxyPort
	}
	mProxyHost, exist := parseStringEnv("PROXY_HOST")
	if exist {
		proxyHost = mProxyHost
	}
	mLocalPort, exist := parseIntEnv("LOCAL_PORT")
	if exist {
		localPort = mLocalPort
	}
	mProtocol, exist := parseStringEnv("PROTOCOL")
	if exist && (mProtocol == "udp" || mProtocol == "tcp") {
		protocol = mProtocol
	}
	mRefreshInterval, exist := parseIntEnv("REFRESH_INTERVAL")
	if exist {
		refresh = mRefreshInterval
	}

	if protocol != "udp" && protocol != "tcp" {
		protocol = "tcp"
	}

	firstRun := true
	oldProxyIpHost := ""

	_, err := os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalf("Failed to create/open config file: %v\n", err)
	}

	for {
		if !firstRun {
			log.Printf("Will refresh again after %d minutes\n", refresh)
			time.Sleep(time.Duration(refresh) * time.Minute)
		} else {
			firstRun = false
		}

		proxyIpHost, err := resolveIpOfHost()
		if err != nil {
			log.Println("Error resolving proxy address:", err)
		} else {
			log.Println("Resolved IP address", "\""+proxyIpHost+"\"", "for host", "\""+proxyHost+"\"")
			if proxyIpHost == oldProxyIpHost {
				log.Printf("Sine IP address is not changed, %s will not be modified\n", "\""+configPath+"\"")
				continue
			}

			err = writeConfig(proxyIpHost)
			if err != nil {
				log.Println("writeConfig error: ", err)
				continue
			}

			err = reloadNginx()
			if err != nil {
				log.Println("reloadNginx error: ", err)
				continue
			}

			oldProxyIpHost = proxyIpHost
		}
	}
}

func writeConfig(proxyIpAddr string) error {
	file, err := os.OpenFile(configPath, os.O_RDWR|os.O_TRUNC, 0644)
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Println("Failed to close file")
		}
	}(file)
	if err != nil {
		log.Println("Failed to open file")
		return err
	}

	filename := strings.TrimSuffix(file.Name(), ".conf")
	template := "upstream %s_ips {\n    server %s:%d;\n}\n\nserver {\n    listen %d %s;\n    proxy_pass %s_ips;\n}\n"
	content := fmt.Sprintf(template, filename, proxyIpAddr, proxyPort, localPort, protocol, filename)
	writer := bufio.NewWriter(file)
	_, err = writer.WriteString(content)
	if err != nil {
		log.Println("Failed to write file")
		return err
	}
	err = writer.Flush()
	if err != nil {
		log.Println("Failed to flush file")
		return err
	}
	return nil
}

func reloadNginx() error {
	cmd := exec.Command("nginx", "-s", "reload")
	err := cmd.Run()
	if err != nil {
		log.Println("Failed to reload nginx config: ", err)
		return err
	}
	log.Println("Nginx config has been reloaded")
	return nil
}

func resolveIpOfHost() (string, error) {
	r, err := net.ResolveIPAddr("ip", proxyHost)
	if err != nil {
		return "", err
	}

	ipAddr := r.String()
	if r.IP.To4() == nil {
		ipAddr = "[" + ipAddr + "]"
	}

	return ipAddr, nil
}
