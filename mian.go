package main

import (
	"log"
	"github.com/holys/goredis"
	"fmt"
	"net/http"
	"os"
	"bufio"
	"io"
	"strings"
	_ "net/http/pprof"
	"time"
)

//output
const (
	stdMode = iota
	rawMode
)

var (
	mode   int
	host   string
	client *goredis.Client
	closeChan chan struct{}
	data map[statistics]int64
	monitorDara map[statistics]int64
)

type statistics struct {
	index 	string  `数据库号`
	ip 	string  `客户端ip`
	option	string  `操作命令`
	param 	[3]string  `参数`
}

func main() {
	config := readConfig()
	for k,v := range config{
		log.Println(k,":",v)
	}
	host = config["host"]
	if host == "" {
		log.Fatalln("请配置host")
	}
	data = make(map[statistics]int64)
	monitorDara = make(map[statistics]int64)
	//indexs := config["index"]

	closeChan = make(chan struct{})

	http.HandleFunc("/start",start)
	http.HandleFunc("/stop",stop)
	http.ListenAndServe(":8080",nil)
}

func start(resp http.ResponseWriter,req *http.Request)  {
	connect()
	go monitor()
	ticker := time.NewTicker(time.Minute * 1)
	go func() {
		for _ = range ticker.C {
			for s,v := range data{
				log.Println("daIndex:",s.index)
				log.Println("		ip:",s.ip)
				log.Println("			option:",s.option)
				log.Println("				count:",v)
			}

		}
	}()
}

func stop(resp http.ResponseWriter,req *http.Request)  {
	defer func() {
		if err:=recover() ; err != nil {
			log.Println("stop err",err)
		}
	}()
	closeChan <- struct{}{}
	//stopChan <- struct{}{}
	client.Close()
}

func connect() {
	if client == nil {
		addr := host
		client = goredis.NewClient(addr, "")
		client.SetMaxIdleConns(1)
		//sendSelect(client, 6)
	}
}

func monitor() {
	respChan := make(chan interface{})
	stopChan := make(chan struct{})
	err := client.Monitor(respChan, stopChan,closeChan)
	if err != nil {
		fmt.Printf("(error) %s\n", err.Error())
		return
	}

	mode = rawMode

	for {
		select {
		case mr := <-respChan:
			printReply(0, mr, mode)
			//fmt.Printf("\n")
		case <-stopChan:
			fmt.Println("Error: Server closed the connection")
			return
		}
	}
}

func readConfig() map[string]string {
	m := make(map[string]string)
	file, err := os.Open("redis_statistics.conf")
	defer file.Close()
	if err != nil {
		log.Println(err)
		return m
	}
	r := bufio.NewReader(file)
	for {
		b, _, err := r.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}
		s := strings.TrimSpace(string(b))
		kv := strings.Split(s, "=")
		m[kv[0]] = kv[1]
	}
	return m
}

func statisticsLog(logs string)  {
	l1 := strings.Split(logs," ")
	if len(l1) < 4{
		return
	}
	var s statistics
	s.index = string([]rune(l1[1])[1:])
	s.ip = string([]rune(l1[2])[0:len([]rune(l1[2]))-1])

	//1492767994.380260 [8 172.16.203.205:57371] "HSET" "/dubbo/com.tinet.crm.rpc.WorkOrderRpcService/consumers" "consumer://172.16.203.205/com.tinet.crm.rpc.WorkOrderRpcService?application=boss&application.version=1.0.0&category=consumers&check=false&default.check=false&default.version=1.0.0&dubbo=2.8.4&interface=com.tinet.crm.rpc.WorkOrderRpcService&methods=notice&pid=3809&revision=1.0.1&side=consumer&timestamp=1488941004252" "1492768053983"
	s.option = l1[3]
	if len(l1) > 4{
		s.param = [3]string{}
		for i:=3;i<len(l1)&&i<6;i++ {
			s.param[i-3] = l1[i]
		}
	}

	data[s] = data[s] + 1
	//log.Println(s)
}

func printReply(level int, reply interface{}, mode int) {
	switch mode {
	case stdMode:
		printStdReply(level, reply)
	case rawMode:
		printRawReply(level, reply)
	default:
		printStdReply(level, reply)
	}

}

func printStdReply(level int, reply interface{}) {
	switch reply := reply.(type) {
	case int64:
		fmt.Printf("(integer) %d", reply)
	case string:
		fmt.Printf("%s", reply)
	case []byte:
		fmt.Printf("%q", reply)
	case nil:
		fmt.Printf("(nil)")
	case goredis.Error:
		fmt.Printf("(error) %s", string(reply))
	case []interface{}:
		for i, v := range reply {
			if i != 0 {
				fmt.Printf("%s", strings.Repeat(" ", level*4))
			}

			s := fmt.Sprintf("%d) ", i+1)
			fmt.Printf("%-4s", s)

			printStdReply(level+1, v)
			if i != len(reply)-1 {
				fmt.Printf("\n")
			}
		}
	default:
		fmt.Printf("Unknown reply type: %+v", reply)
	}
}

func printRawReply(level int, reply interface{}) {
	switch reply := reply.(type) {
	case int64:
		fmt.Printf("%d --------1", reply)
	case string:
		{
			//if strings.Contains(reply, "HSET") {
			//fmt.Printf("%s", reply)
			//fmt.Println()
			statisticsLog(reply)
			//}
		}
	case []byte:
		fmt.Printf("%s --------2", reply)
	case nil:
		// do nothing
	case goredis.Error:
		fmt.Printf("%s\n --------3", string(reply))
	case []interface{}:
		for i, v := range reply {
			if i != 0 {
				fmt.Printf("%s  --------4", strings.Repeat(" ", level*4))
			}

			printRawReply(level+1, v)
			if i != len(reply)-1 {
				fmt.Println("--------5")
			}
		}
	default:
		fmt.Printf("Unknown reply type: %+v", reply)
	}
}

func sendSelect(client *goredis.Client, index int) {
	if index == 0 {
		// do nothing
		return
	}
	if index > 16 || index < 0 {
		index = 0
		fmt.Println("index out of range, should less than 16")
	}
	_, err := client.Do("SELECT", index)
	fmt.Println("SELECT", index)
	_, err = client.Do("set", "test", "111")
	if err != nil {
		fmt.Printf("%s\n", err.Error())
	}
}

func sendAuth(client *goredis.Client, passwd string) error {
	if passwd == "" {
		// do nothing
		return nil
	}

	resp, err := client.Do("AUTH", passwd)
	if err != nil {
		fmt.Printf("(error) %s\n", err.Error())
		return err
	}

	switch resp := resp.(type) {
	case goredis.Error:
		fmt.Printf("(error) %s\n", resp.Error())
		return resp
	}

	return nil
}
