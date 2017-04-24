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
	"strconv"
	"regexp"
	"encoding/json"
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
	stopTicket chan int
	data map[statistics]int64
	monitorDara map[statistics]*tally
	saveIndex int
	regexps string
	reg *regexp.Regexp
	lock chan int
	started bool
)

type statistics struct {
	index 	string  `数据库号`
	ip 	string  `客户端ip`
	option	string  `操作命令`
	param 	[3]string  `参数`
}

type tally struct {
	entity bool
	//option string `命令`
	totalCount int64 `总数`
	count int64  `根据正则表达式匹配总数`
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
	sIndex := config["saveToIndex"]
	if sIndex == ""{
		sIndex = "0"
	}
	saveIndex,_ = strconv.Atoi(sIndex)
	log.Println("saveIndex",saveIndex)
	regexps = config["regexp"]
	if regexps!= ""{
		reg = regexp.MustCompile(regexps)
	}
	httpPort := config["httpPort"]
	if httpPort == ""{
		httpPort = "8080"
	}
	buildMonitorData(config)//初始化需要监控的数据
	data = make(map[statistics]int64)
	stopTicket = make(chan int)
	closeChan = make(chan struct{})
	lock = make(chan int,1)
	http.HandleFunc("/start",start)
	http.HandleFunc("/stop",stop)
	http.ListenAndServe(":"+httpPort,nil)
}

func buildMonitorData(config map[string]string)  {
	monitorDara = make(map[statistics]*tally)
	var dbindexs []string
	var ips []string
	var options []string

	indexs := config["index"]
	if indexs != ""{
		dbindexs = strings.Split(indexs,",")
	}
	if len(dbindexs)==0{//没配置index，默认统计所有
		dbindexs = []string{"0","1","2","3","4","5","6","7","8","9","10","11","12","13","14","15"}
	}
	is := config["ip"]
	if is != ""{
		ips = strings.Split(is,",")
	}
	ops := config["options"]

	if ops != ""{
		options = strings.Split(ops,",")
	}

	if ops == "" || len(options)==0{
		log.Fatalln("请配置options,多个以,分开")
	}

	for _,index := range dbindexs{

		if len(ips) == 0{
			for _,option := range options {
				var s statistics = statistics{}
				s.index = index
				s.option = option
				monitorDara[s] = &tally{true,0,0}
				log.Println("1",s)
			}
		}else {
			for _,ip := range ips{

				for _,option := range options {
					var s statistics = statistics{}
					s.index = index
					s.ip = ip
					s.option = option
					monitorDara[s] = &tally{true,0,0}
					//log.Println("2",s)
				}

			}
		}

	}
}

func start(resp http.ResponseWriter,req *http.Request)  {
	defer func() {<-lock}()
	lock <- 1
	if started{
		log.Println("mointor has start")
		return
	}
	started = true
	connect()
	go monitor()
	go saveStatistics()
	log.Println("start monitor")
}

func saveStatistics()  {
	ticker := time.NewTicker(time.Minute * 1)
	for {
		select {
		case <-ticker.C:{
			statises := []Statis{}
			for s,v := range monitorDara{
				log.Println("daIndex:",s.index)
				log.Println("		ip:",s.ip)
				log.Println("			option:",s.option)
				log.Println("				count:",v)
				if v.totalCount > 0 {
					statises = append(statises,Statis{s.index,s.ip,s.option,strconv.FormatInt(v.totalCount,10),strconv.FormatInt(v.count,10)})
				}
			}
			sendSelect(client,saveIndex)
			json,_:=json.Marshal(statises)
			cmds := []string{"set","redis_statistics",string(json)}
			SendCommand(cmds)
			timeout := 60*60 //单位秒
			cmds = []string{"expire","redis_statistics",strconv.Itoa(timeout)}
			SendCommand(cmds)
		}
		case <-stopTicket :{
			log.Println("stop ticker")
			return
		}
		}
	}
}

type Statis struct {
	Dbindex string
	Ip string
	Option string
	TotalCount string
	Count string
}

func stop(resp http.ResponseWriter,req *http.Request)  {
	defer func() {
		if err:=recover() ; err != nil {
			log.Println("stop err",err)
		}
	}()
	defer func() {<-lock}()
	lock <- 1
	if !started{
		log.Println("mointor has stopped")
		return
	}
	started = false
	closeChan <- struct{}{}
	stopTicket <- 1
	client.Close()
}

func connect() {
	if client == nil {
		addr := host
		client = goredis.NewClient(addr, "")
		client.SetMaxIdleConns(1)
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
		if len(kv)==2{
			m[kv[0]] = kv[1]
		}
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
	s.option = strings.Replace(l1[3],"\"","",-1)

	defer func() {
		if err:=recover();err!=nil{
			log.Println("err",err)
		}
	}()
	if mdata,ok := monitorDara[s];ok{
		mdata.totalCount = mdata.totalCount + 1  //记录操作总数
		if len(l1) > 4{
			for i:=3;i<len(l1)&&i<6;i++ {
				var param string = l1[i]
				if finsStr :=reg.FindString(param); finsStr!= ""{
					mdata.count = mdata.count + 1
					log.Println("regexp:",finsStr)
					//log.Println("reg",param)
					break
				}
			}
		}
	}
	s.ip = string([]rune(l1[2])[0:len([]rune(l1[2]))-1])
	if mdata,ok := monitorDara[s];ok{
		mdata.totalCount = mdata.totalCount + 1  //记录操作总数
		if len(l1) > 4{
			for i:=3;i<len(l1)&&i<6;i++ {
				var param string = l1[i]
				if finsStr :=reg.FindString(param); finsStr!= ""{
					mdata.count = mdata.count + 1
					log.Println("regexp:",finsStr)
					//log.Println("with ip reg",param)
					break
				}
			}
		}
	}
	/*if len(l1) > 4{  //set param
		s.param = [3]string{}
		for i:=3;i<len(l1)&&i<6;i++ {
			s.param[i-3] = l1[i]
		}
	}*/
	//data[s] = data[s] + 1
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
			statisticsLog(reply)
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

func SendCommand(cmds []string) {
	if len(cmds) == 0 {
		return
	}
	args := make([]interface{}, len(cmds[1:]))
	for i := range args {
		args[i] = strings.Trim(string(cmds[1+i]), "\"'")
	}

	cmd := strings.ToLower(cmds[0])

	r, err := client.Do(cmd, args...)

	if err != nil {
		log.Printf("(error) %s", err.Error())
	} else {
		log.Println(r)
	}
}
