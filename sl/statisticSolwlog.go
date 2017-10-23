package sl

import (
	"log"
	"strconv"
	"github.com/holys/goredis"
	"reflect"
	"go/types"
	"sort"
	"os"
	"encoding/json"
	"strings"
	"time"
	"fmt"
)

var start bool = false

type slowLog struct {
	UniqueId int64  	`id`
	UnixTime int64      `时间戳`
	Time     int64      `执行时间`
	Commonds []string   `操作`
}

func StartMonitorSlowlog(addresses string)  {
	addrs := strings.Split(addresses,";")
	if !start && len(addrs)>0{
		start = true
		go func() {
			ticker := time.NewTicker(time.Second * 10)
			for {
				select {
				case <-ticker.C:
					for _,addr := range addrs{
						statisticSlowlog(addr)
					}
				}
			}
		}()
	}
}

func getSlowlog(client *goredis.Client,logger *log.Logger) []slowLog  {
	r, err := client.Do("get", "slowlogs")
	if err != nil{
		logger.Println("getSlowlog err:",err)
	}
	slowLogs := fmt.Sprintf("%s", r)
	//logger.Println(slowLogs)
	slowlog := make([]slowLog,0,0)
	err = json.Unmarshal([]byte(slowLogs),&slowlog)
	if err != nil{
		logger.Println("Unmarshal err:",err)
	}
	//logger.Println("olgSlowlogs:",slowlog)
	return slowlog
}

func statisticSlowlog(addr string)  {
	file,err := os.OpenFile("monitorSlowlog.log",os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil{
		fmt.Println(err)
	}
	var logger *log.Logger = log.New(file,"[info]",log.LstdFlags)
	var client *goredis.Client = goredis.NewClient(addr, "",logger)
	client.SetMaxIdleConns(1)
	defer client.Close()
	sendSelect(client,9,logger)
	slowLogs := getSlowLogs(client,logger)
	if len(slowLogs) > 100{
		slowLogs = slowLogs[0:100]
	}
	bs,_:=json.Marshal(slowLogs)
	r,err := client.Do("set","slowlogs",string(bs))
	if err != nil{
		logger.Println("set slowlogs err:",err)
	}
	logger.Println("set slowlogs:",r)
	r,err = client.Do("expire","slowlogs",60 * 60 * 60)
	if err != nil{
		logger.Println("expire slowlogs err:",err)
	}
	logger.Println("expire slowlogs:",r)
}

func getSlowLogs(client *goredis.Client,logger *log.Logger) []slowLog {
	r, err := client.Do("slowlog", "get","10000")
	if err != nil{
		logger.Println("err:",err)
	}
	commonds := make([]string,0,0)
	logger.Println(reflect.TypeOf(r).String())
	switch reply := r.(type) {
	case string:
		logger.Println("string")
	case types.Slice:
		logger.Println("slice")
	case types.Map:
		logger.Println("map")
	case types.Array:
		logger.Println("array")
	case []interface{}:
		commonds = chuli(reply,commonds,logger)
	default:
		logger.Println(reply)
	}
	tag := 0
	slowlogs := make([]slowLog,0,0)
	var slowlog slowLog
	var com []string
	for _,c := range commonds{
		if "start" == c && tag == 0 {
			slowlog = *new(slowLog)
			tag++
		}else if "start" == c && tag > 0{
			com = make([]string,0,0)
			tag = -1
		}else if "end" == c && tag == -1{
			slowlog.Commonds = com
			tag = -2
		}else if "end" == c && tag == -2{
			slowlogs = append(slowlogs,slowlog)
			tag = 0
		}else if tag == 1{
			slowlog.UniqueId,_ = strconv.ParseInt(c,10,64)
			tag++
		}else if tag == 2{
			slowlog.UnixTime,_ = strconv.ParseInt(c,10,64)
			tag++
		}else if tag == 3{
			slowlog.Time,_ = strconv.ParseInt(c,10,64)
			tag++
		}else if tag < 0{
			com = append(com,c)

		}

	}
	oldSlowlogs := getSlowlog(client,logger)
	slowlogs = append(slowlogs,oldSlowlogs...)
	sort.Slice(slowlogs, func(i, j int) bool {
		return slowlogs[i].Time > slowlogs[j].Time
	})
	//logger.Println("commonds:",commonds)
	if len(slowlogs) > 10{
		logger.Println("slowlogs:",slowlogs[0:10])
	}else {
		logger.Println("slowlogs:",slowlogs)
	}

	r, err = client.Do("slowlog", "reset")
	if err != nil{
		logger.Println("err:",err)
	}
	logger.Println(r)
	return slowlogs
}

func chuli(arg []interface{},commonds []string,logger *log.Logger) []string {
	for _,v := range arg{
		//logger.Println("属性",i,"的值是",v)
		switch vtype:= v.(type) {
		case []interface{}:{
			commonds = append(commonds,"start")
			commonds = chuli(vtype,commonds,logger)
			commonds = append(commonds,"end")
		}
		case int64:
			//log.Println("int64:",)
			commonds = append(commonds,strconv.FormatInt(vtype,10))
		case string:logger.Println("string")
		case []byte:
			commonds = append(commonds,string(vtype))
		}
	}
	return commonds
}

func sendSelect(client *goredis.Client, index int,logger *log.Logger) {
	defer func() {
		if err:=recover();err!=nil{
			logger.Println("sendSelect.err,",err)
			return
		}
	}()
	if index == 0 {
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
