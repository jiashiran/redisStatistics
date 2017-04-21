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
)

func main() {
	config := readConfig()
	host = config["host"]
	if host == "" {
		log.Fatalln("请配置host")
	}

	closeChan = make(chan struct{})

	http.HandleFunc("/start",start)
	http.HandleFunc("/stop",stop)
	http.ListenAndServe(":8080",nil)
}

func start(resp http.ResponseWriter,req *http.Request)  {
	connect()
	go monitor()
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
			fmt.Printf("%s", reply)
			fmt.Println()
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
