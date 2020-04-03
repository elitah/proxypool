package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/elitah/proxypool/pool"
	"github.com/elitah/socks"
	"github.com/elitah/utils/logs"
)

type myCheker struct {
	address string

	flags uint32
}

func (this *myCheker) IsExit() bool {
	return false
}

func (this *myCheker) GetAddress() (string, string) {
	return "tcp", this.address
}

func (this *myCheker) CheckConn(conn net.Conn) bool {
	var buffer [1024]byte
	//
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	//
	if n, err := conn.Read(buffer[:]); nil == err {
		if 0 < n {
			return strings.Contains(string(buffer[:n]), "SSH")
		} else {
			logs.Error("empty recv")
		}
	} else {
		//
		if !errors.As(err, &io.EOF) {
			logs.Error(err)
		}
	}

	return false
}

func (this *myCheker) ShowError(err error) {
	//
	if !errors.As(err, &io.EOF) {
		logs.Error(err)
	}
}

func main() {
	logs.SetLogger(logs.AdapterConsole, `{"level":99,"color":true}`)
	logs.EnableFuncCallDepth(true)
	logs.SetLogFuncCallDepth(3)
	logs.Async()

	defer logs.Close()

	pm := pool.NewProxyManager()

	cc := &myCheker{address: "elitah.xyz:22"}

	list := []string{
		"222.96.237.2:4145",
		"59.6.180.232:4145",
		"221.154.72.249:4145",
		"103.38.25.206:4145",
		"112.161.211.65:4145",
		"14.33.75.22:4145",
		"211.35.73.30:4145",
		"103.38.25.226:4145",
		"59.31.90.206:4145",
		"106.245.183.58:4145",
		"58.149.49.186:4145",
		"61.39.130.75:4145",
		"211.35.72.126:4145",
		"211.114.178.122:4145",
		"112.220.151.204:4145",
		"1.223.248.99:4145",
		"175.195.33.102:4145",
		"106.242.87.138:4145",
		"121.66.154.171:4145",
		"211.35.73.254:4145",
		"125.138.129.101:4145",
		"61.32.154.211:4145",
		"1.220.145.45:4145",
		"103.38.25.210:4145",
		"103.38.25.142:4145",
		"211.114.178.168:4145",
		"124.194.83.172:4145",
		"1.215.122.108:4145",
		"115.94.207.204:4145",
		"121.138.155.41:4145",
		"1.221.173.148:4145",
		"211.35.72.46:4145",
		"103.38.25.218:4145",
		"123.143.77.180:4145",
		"1.212.181.131:4145",
	}

	fmt.Println("hello")

	for _, item := range list {
		if node := pool.NewProxyNode(
			socks.Dial(fmt.Sprintf("socks4://%s?timeout=3s", item)),
		); nil != node {
			node.StartCheck(cc)

			pm.Store(item, node)
		}
	}

	for {
		time.Sleep(3 * time.Second)

		list := pm.GetSortList()

		for i, item := range list {
			fmt.Printf("[%02d] %v\n", i, item)
		}

		httpClient := &http.Client{
			Transport: &http.Transport{
				Dial:            list[0].Dial,
				MaxIdleConns:    1,
				IdleConnTimeout: 1 * time.Second,
			},
		}

		if resp, err := httpClient.Get("http://ddns.oray.com/checkip"); nil == err {
			if resp.StatusCode == http.StatusOK {
				if buf, err := ioutil.ReadAll(resp.Body); nil == err {
					logs.Info(string(buf))
				} else {
					logs.Error(err)
				}
			} else {
				logs.Warn(resp.StatusCode)
			}
			//
			resp.Body.Close()
		} else {
			logs.Error(err)
		}

		fmt.Println("\n\n\n")
	}
}
