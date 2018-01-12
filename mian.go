package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/holys/goredis"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"redisStatistics/sl"
	"regexp"
	"strconv"
	"strings"
	"time"
	"sort"
	"sync"
	"runtime"
	"sync/atomic"
	"redisStatistics/utils"
)

//output
const (
	stdMode = iota
	rawMode
)

var (
	mode        int
	host        string
	client      *goredis.Client
	respChan    chan interface{}
	stopTicket  chan int
	data        map[statistics]int64
	monitorDara *sync.Map
	saveIndex   int
	regexps     string
	reg         []*regexp.Regexp
	lock        chan int
	started     bool
	debug       bool
	logger      *log.Logger
	httpPort    string
	config      map[string]string  //conf中的配置数据
	startTime   string
	operateSum  *int64
	queueSize   = 1000000
	queue		chan string
	mapLock     sync.Mutex
	matchLock   sync.Mutex
	statisticsGrotuneCount int
	statisticsStopChan	chan int
)

type statistics struct {
	index  string    `数据库号`
	ip     string    `客户端ip`
	option string    `操作命令`
	param  [3]string `参数`
}

type tally struct {
	entity bool  //标记是否是通过配置需要匹配的统计，非配置里的也会统计值是false
	//option string `命令`
	totalCount *int64            `总数`
	count      *sync.Map //map[string]int64 `根据正则表达式匹配总数`
}

func main() {
	buildMonitorData(config) //初始化需要监控的数据
	data = make(map[statistics]int64)
	stopTicket = make(chan int)
	lock = make(chan int, 1)
	queue = make(chan string,queueSize)
	http.HandleFunc("/start", start)
	http.HandleFunc("/stop", stop)
	http.HandleFunc("/info", info)
	http.HandleFunc("/startMonitorSlowlog", startMonitorSlowlog)
	http.HandleFunc("/getSlowlog", getSlowlog)
	http.ListenAndServe(":"+httpPort, nil)

}

func startMonitorSlowlog(resp http.ResponseWriter, req *http.Request) {
	sl.StartMonitorSlowlog(config["slowlogAddrs"])
	io.WriteString(resp, "开始统计slowlog")
}

func getSlowlog(resp http.ResponseWriter, req *http.Request) {
	file, err := os.OpenFile("monitorSlowlog.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println(err)
	}
	var logger *log.Logger = log.New(file, "[info]", log.LstdFlags)
	addrsConfig := config["slowlogAddrs"]
	addrs := strings.Split(addrsConfig, ";")
	for _, addr := range addrs {
		var client *goredis.Client = goredis.NewClient(addr, "", logger)
		client.SetMaxIdleConns(1)
		defer client.Close()
		sendSelect(client, 9)
		r, err := client.Do("get", "slowlogs")
		if err != nil {
			log.Println("getSlowlog err:", err)
		}
		io.WriteString(resp, addr+"："+fmt.Sprintf("%s", r))
	}
}

func init() {
	statisticsStopChan = make(chan int)
	Init()
}

func Init()  {
	config = readConfig()
	logFlag := config["logFlag"]
	operateSum = utils.Int64(0)
	// 创建一个日志对象
	if logFlag != "" && logFlag == "file" {
		// 定义一个文件
		fileName := "redis_statistics.log"
		logFile, err := os.Create(fileName)
		//defer logFile.Close()
		if err != nil {
			log.Println("open file error !")
		}
		logger = log.New(logFile, "[info]", log.LstdFlags)
	} else {
		logger = log.New(os.Stdout, "[info]", log.LstdFlags)
	}

	for k, v := range config {
		logger.Println(k, ":", v)
	}
	host = config["host"]
	if host == "" {
		log.Fatalln("请配置host")
	}
	sIndex := config["saveToIndex"]
	if sIndex == "" {
		sIndex = "0"
	}
	saveIndex, _ = strconv.Atoi(sIndex)
	logger.Println("saveIndex", saveIndex)
	regexps = config["regexp"]
	if regexps != "" {
		regs := strings.Split(regexps, ";")
		reg = make([]*regexp.Regexp, 0)
		for _, r := range regs {
			reg = append(reg, regexp.MustCompile(r))
		}
	}
	logger.Println("regexps:", reg)
	httpPort = config["httpPort"]
	if httpPort == "" {
		httpPort = "8080"
	}

	if logFlag != "" && logFlag == "debug" {
		logger.SetPrefix("[debug]")
		debug = true
	}
}



func buildMonitorData(config map[string]string) {
	monitorDara = new(sync.Map)
	var dbindexs []string
	var ips []string
	var options []string

	indexs := config["index"]
	if indexs != "" {
		dbindexs = strings.Split(indexs, ",")
	}
	if len(dbindexs) == 0 { //没配置index，默认统计所有
		dbindexs = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15"}
	}
	is := config["ip"]
	if is != "" {
		ips = strings.Split(is, ",")
	}
	ops := config["options"]

	if ops != "" {
		options = strings.Split(strings.ToLower(ops), ",")
	}

	if ops == "" || len(options) == 0 {
		log.Fatalln("请配置options,多个以,分开")
	}

	for _, index := range dbindexs {

		if len(ips) == 0 {
			for _, option := range options {
				var s statistics = statistics{}
				s.index = index
				s.option = option
				counts := new(sync.Map)
				monitorDara.Store(s,&tally{true, utils.Int64(0), counts})
				logger.Println("1", s)
			}
		} else {
			for _, ip := range ips {

				for _, option := range options {
					var s statistics = statistics{}
					s.index = index
					s.ip = ip
					s.option = option
					counts := new(sync.Map)
					monitorDara.Store(s,&tally{true, utils.Int64(0), counts})
					//logger.Println("2",s)
				}

			}
		}

	}
}

func info(resp http.ResponseWriter, req *http.Request) {
	addr := host
	infoClient := goredis.NewClient(addr, "", logger)
	infoClient.SetMaxIdleConns(1)
	sendSelect(infoClient, saveIndex)
	cmds := []string{"get", "redis_statistics"}
	r, err := SendCommand(infoClient,cmds)
	if err != nil {
		log.Println(err)
	}
	//value := reflect.ValueOf(r)
	//logger.Println(value)
	infoClient.Close()
	io.WriteString(resp, fmt.Sprintf("%s", r))
}

func start(resp http.ResponseWriter, req *http.Request) {
	defer func() { <-lock }()
	lock <- 1
	if started {
		logger.Println("mointor has start")
		io.WriteString(resp, "已打开统计")
		return
	}
	started = true
	Init()
	buildMonitorData(config)
	runtime.GC()
	connect()
	go monitor()
	go saveStatistics()
	cpuNum := runtime.NumCPU()
	handlerLogRoutineCount := config["handlerLogRoutineCount"]
	if handlerLogRoutineCount != ""{
		var err error
		cpuNum,err = strconv.Atoi(handlerLogRoutineCount)
		if err != nil{
			log.Fatalln("handlerLogRoutineCount is error:",err)
		}
	}
	log.Println("cpuNum:",cpuNum)
	statisticsGrotuneCount = 0
	for i:=1;i<=cpuNum;i++{
		go func(index int) {
			statisticsGrotuneCount++
			log.Println("start new statistics goroutine:",index)
			for{
				select {
				case v:= <- queue:{
					if !started {
						break
					}
					statisticsLog(v)
				}
				case i := <- statisticsStopChan:{
					log.Println("stop statistics goroutine:",i)
					return
				}
				}
			}
		}(i)
	}
	logger.Println("start monitor")
	startTime = time.Now().Format("2006-01-02 15:04:05")
	io.WriteString(resp, "已打开统计")

}

func stop(resp http.ResponseWriter, req *http.Request) {
	if !started {
		logger.Println("mointor has stopped")
		io.WriteString(resp, "已关闭统计1")
		return
	}
	close(respChan)
	defer func() {
		log.Println("statisticsGrotuneCount:",statisticsGrotuneCount)
		for  ; statisticsGrotuneCount > 0 ; statisticsGrotuneCount--{
			statisticsStopChan <- statisticsGrotuneCount
		}
	}()
	defer func() {
		if err := recover(); err != nil {
			logger.Println("stop info", err)
		}
	}()
	defer func() {
		<-lock
		client = nil
		io.WriteString(resp, "已关闭统计")
	}()
	lock <- 1
	started = false
	stopTicket <- 1
	sendSelect(client, saveIndex)
	timeout := 60 * 60 //单位秒
	buildAndSave()
	cmds := []string{"expire", "redis_statistics", strconv.Itoa(timeout)}
	SendCommand(client,cmds)
	client.Close()
	client = nil
}

func saveStatistics() {
	ticker := time.NewTicker(time.Minute * 1)
	for {
		select {
		case <-ticker.C:
			{
				buildAndSave()

			}
		case <-stopTicket:
			{
				logger.Println("stop ticker")
				ticker.Stop()
				return
			}
		}
	}
}

func buildAndSave()  {
	logger.Println("start save statistic")
	sum := utils.Int64Value(operateSum)
	statises := getStatisticsData(*monitorDara)
	if len(statises) == 0{
		return
	}
	sort.SliceStable(statises, func(i, j int) bool {return statises[i].TotalCount > statises[j].TotalCount})

	body := JsonBody{
		StartTime:       startTime,
		EndTime:         time.Now().Format("2006-01-02 15:04:05"),
		OperateSumCount: sum,
		Regexp:          regexps, Data: statises,
	}
	json, _ := json.Marshal(body)
	cmds := []string{"set", "redis_statistics", string(json)}
	addr := host
	infoClient := goredis.NewClient(addr, "", logger)
	infoClient.SetMaxIdleConns(1)
	sendSelect(infoClient, saveIndex)
	SendCommand(infoClient,cmds)
	infoClient.Close()
	infoClient = nil
}

func getStatisticsData(monitorDara sync.Map)[]Statis  {
	statises := []Statis{}
	monitorDara.Range(func(key, value interface{}) bool {
		s,_ := key.(statistics)
		v,_ := value.(*tally)
		if debug {
			logger.Println("daIndex:", s.index)
			logger.Println("		ip:", s.ip)
			logger.Println("			option:", s.option)
			logger.Println("				count:", v)
		}
		if *v.totalCount > 0 {
			countMap := make(map[string]int64)

			v.count.Range(func(c_key, c_value interface{}) bool {
				countMap[c_key.(string)] = utils.Int64Value(c_value.(*int64))
				return true
			})
			statises = append(statises, Statis{s.index, s.ip, s.option, *v.totalCount, countMap})
			if !debug {
				logger.Println("daIndex:", s.index)
				logger.Println("		ip:", s.ip)
				logger.Println("			option:", s.option)
				logger.Println("				count:", v)
			}
		}
		return true
	})
	return statises
}

type JsonBody struct {
	StartTime       string
	EndTime         string
	Regexp          string
	OperateSumCount int64
	Data            []Statis
}

type Statis struct {
	Dbindex    string
	Ip         string
	Option     string
	TotalCount int64
	Regexps    map[string]int64
}




func connect() {
	if client == nil {
		addr := host
		client = goredis.NewClient(addr, "", logger)
		client.SetMaxIdleConns(1)
	}
}

func monitor() {
	respChan = make(chan interface{})
	stopChan := make(chan struct{})
	err := client.Monitor(respChan, stopChan)
	if err != nil {
		logger.Printf("(error) %s\n", err.Error())
		return
	}

	mode = rawMode
	for {
		select {
		case mr := <-respChan:
			printReply(0, mr, mode)
			//logger.Printf("\n")
		case <- stopChan:
			logger.Println("Error: Server closed the connection")
			started = false
			close(stopChan)
			return
		}
	}

}

func readConfig() map[string]string {
	m := make(map[string]string)
	file, err := os.Open("statistics.conf")
	defer file.Close()
	if err != nil {
		logger.Println(err)
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
		if len(kv) == 2 {
			m[kv[0]] = kv[1]
		}
	}
	return m
}

func statisticsLog(logs string) {
	logs = strings.ToLower(logs)
	if logs == "" {
		return
	}
	l1 := strings.Split(logs, " ")
	if len(l1) < 4 {
		log.Println("error log:",l1)
		return
	}
	atomic.AddInt64(operateSum , 1)
	var s statistics
	s.index = string([]rune(l1[1])[1:])
	s.option = strings.Replace(l1[3], "\"", "", -1)

	defer func() {
		if err := recover(); err != nil {
			logger.Println("err", err)
		}
	}()
	if value, ok := monitorDara.Load(s); ok {
		mdata,_ := value.(*tally)
		matchFilter(mdata,l1)
	}else if config["mode"] == "all" {//根据配置未匹配到日志，新建一项统计，entity=false
		mapLock.Lock()
		if value, ok := monitorDara.Load(s); ok {
			mdata,_ := value.(*tally)
			matchFilter(mdata,l1)
		}else {
			mdata := &tally{false, utils.Int64(0), new(sync.Map)}
			matchFilter(mdata,l1)
			monitorDara.Store(s,mdata)
		}
		mapLock.Unlock()
	}

	s.ip = string([]rune(l1[2])[0 : len([]rune(l1[2]))-1])
	if value, ok := monitorDara.Load(s); ok {
		mdata,_ := value.(*tally)
		matchFilter(mdata,l1)
	}
	/*if len(l1) > 4{  //set param
		s.param = [3]string{}
		for i:=3;i<len(l1)&&i<6;i++ {
			s.param[i-3] = l1[i]
		}
	}*/
	//data[s] = data[s] + 1
	//logger.Println(s)
	if debug {
		if len(l1) > 4 { //set param
			s.param = [3]string{}
			for i := 3; i < len(l1) && i < 6; i++ {
				s.param[i-3] = l1[i]
			}
		}
		logger.Println(s)
	}
}

func matchFilter(mdata *tally,l1 []string){
	atomic.AddInt64(mdata.totalCount , 1) //记录操作总数
	if mdata.entity && len(l1) > 4 {
		for i := 3; i < len(l1) && i < 6; i++ {
			var param string = l1[i]
			for _, rege := range reg {
				//logger.Println(rege,param)
				if finsStr := rege.FindString(param); finsStr != "" {
					countMap := mdata.count
					countMap_value,ok := countMap.Load(rege.String())
					if ok{
						atomic.AddInt64(countMap_value.(*int64),1)
					}else {
						matchLock.Lock()
						if countMap_value,ok := countMap.Load(rege.String());ok{
							atomic.AddInt64(countMap_value.(*int64),1)
						}else {
							countMap.Store(rege.String(),utils.Int64(1))
						}
						matchLock.Unlock()
					}
					if debug {
						logger.Println("regexp:", finsStr)
					}
					//logger.Println("reg",param)
					break
				}
			}
		}
	}
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
		logger.Printf("(integer) %d", reply)
	case string:
		logger.Printf("%s", reply)
	case []byte:
		logger.Printf("%q", reply)
	case nil:
		logger.Printf("(nil)")
	case goredis.Error:
		logger.Printf("(error) %s", string(reply))
	case []interface{}:
		for i, v := range reply {
			if i != 0 {
				logger.Printf("%s", strings.Repeat(" ", level*4))
			}

			s := fmt.Sprintf("%d) ", i+1)
			logger.Printf("%-4s", s)

			printStdReply(level+1, v)
			if i != len(reply)-1 {
				logger.Printf("\n")
			}
		}
	default:
		{
			logger.Printf("Unknown reply type 0: %+v", reply)
			os.Exit(0)
		}
	}
}

func printRawReply(level int, reply interface{}) {
	switch reply := reply.(type) {
	case int64:
		logger.Printf("%d --------1", reply)
	case string:
		{
			//statisticsLog(reply)
			if len(queue) < queueSize{
				queue <- reply
			}else {
				log.Println("queue if full,log:",reply)
			}
		}
	case []byte:
		logger.Printf("%s --------2", reply)
	case nil:
		// do nothing
	case goredis.Error:
		logger.Printf("%s\n --------3", string(reply))
	case []interface{}:
		for i, v := range reply {
			if i != 0 {
				logger.Printf("%s  --------4", strings.Repeat(" ", level*4))
			}

			printRawReply(level+1, v)
			if i != len(reply)-1 {
				logger.Println("--------5")
			}
		}
	case error:
		{
			logger.Println("printRawReply error:", reply)
			if strings.Contains(reply.Error(), "use of closed network connection") {
				time.Sleep(time.Second * 10)
				go monitor()
				log.Println("重新建立连接")
			}
		}
	default:
		{
			logger.Printf("Unknown reply type 1: %+v", reply)
			os.Exit(0)
		}

	}
}

func sendSelect(client *goredis.Client, index int) {
	defer func() {
		if err := recover(); err != nil {
			logger.Println("sendSelect.err,", err)
			return
		}
	}()
	if index == 0 {
		// do nothing
		return
	}
	if index > 16 || index < 0 {
		index = 0
		logger.Println("index out of range, should less than 16")
	}
	_, err := client.Do("SELECT", index)
	logger.Println("SELECT", index)
	if err != nil {
		logger.Printf("%s\n", err.Error())
	}
}

func sendAuth(client *goredis.Client, passwd string) error {
	if passwd == "" {
		// do nothing
		return nil
	}

	resp, err := client.Do("AUTH", passwd)
	if err != nil {
		logger.Printf("(error) %s\n", err.Error())
		return err
	}

	switch resp := resp.(type) {
	case goredis.Error:
		logger.Printf("(error) %s\n", resp.Error())
		return resp
	}

	return nil
}

func SendCommand(client *goredis.Client,cmds []string) (interface{}, error) {
	if len(cmds) == 0 {
		return nil, errors.New("agrs is null")
	}
	args := make([]interface{}, len(cmds[1:]))
	for i := range args {
		args[i] = strings.Trim(string(cmds[1+i]), "\"'")
	}

	cmd := strings.ToLower(cmds[0])

	r, err := client.Do(cmd, args...)

	if err != nil {
		logger.Printf("(error) %s", err.Error())
	} else {
		//logger.Println(r)
	}
	return r, err
}
