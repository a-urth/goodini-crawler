package main

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

var once sync.Once
var proxy Proxy

type Proxy struct {
	proxies chan string
}

func LoadProxies(fileName string) {
	file, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	var t []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		url := fmt.Sprintf("http://%s:3128", scanner.Text())
		t = append(t, url)
	}
	proxy.proxies = make(chan string, len(t))
	for _, p := range t {
		proxy.proxies <- p
	}
}

func GetProxy() string {
	return <-proxy.proxies
}

func ReleaseProxy(p string) {
	proxy.proxies <- p
}
