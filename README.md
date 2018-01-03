# redisStatistics
redis日志统计，统计redis所有操作的次数或只统计配置中的操作，结果按操作总次数从大到小排序，统计slowlog

编译说明：通过交叉编译，可以编译得到3种操作系统的版本

windows：环境变量 GOOS=windows，GOARCH=amd64

```
go build -o redisStatistics-windiws-amd64
```

linux：环境变量 GOOS=linux，GOARCH=amd64

```
go build -o redisStatistics-linux-amd64
```

mac：环境变量 GOOS=darwin，GOARCH=amd64

```
go build -o redisStatistics-mac-amd64
```

配置说明：

| 字段          | 可选   | 描述                                       | 例子                  |
| ----------- | ---- | ---------------------------------------- | ------------------- |
| host        | 必选   | redis地址                                  | 172.16.203.193:6379 |
| mode        | 必选   | 监控模式，all是所有操作， config是只监控配置中的  | all |
| options     | 必选   | 操作指令，多个用“,”区分                            | HGET,HSET           |
| ip          | 可选   | 请求来自哪些ip，多个用“,”区分，不配置统计所有来源              | 172.16.203.193      |
| index       | 可选   | 数据库索引，多个用“,”区分，不配置统计所有数据库                | 1,2,3,4,5,6,7,8     |
| regexp      | 可选   | 操作参数匹配的正则表达式，不配置统计所有,支持多个正则匹配，用;分隔                     | monitor;scheduler;queue_avalible_member;8000;queue             |
| saveToIndex | 可选   | 统计结果保存的redis数据库，不配置默认为0                  | 2                   |
| httpPort    | 可选   | 控制打开或关闭监控的rest接口的端口，不配置默认为8080           |                     |
| logFlag     | 可选   | 日志模式，可配置info,debug,file，不配置默认为info,file模式日志输出到文件 |                     |
| handlerLogRoutineCount     | 可选   | 处理日志的goroutine数量，默认是cpu的核数 |                     |


打开监控：

```
http://172.16.203.194:8080/start

```

关闭监控：

```
http://172.16.203.194:8080/stop

```

查看统计信息：
```
http://172.16.203.194:8080/info

```

打开统计slowlog：
```
http://172.16.203.194:8080/startMonitorSlowlog

```

查看slowlog信息：
```
http://172.16.203.194:8080/getSlowlog

```

监控会将结果以json格式存入指定数据库中，key为“redis_statistics”，数据每分钟同步一次，停止监控空后数据有效期为1小时

返回结果字段说明：

| 字段名         | 描述   |
| ----------- | ---- |
| StartTime        | 统计开始时间   |
| EndTime        | 统计截止时间   |
| Regexp     | 需要匹配正则表达式   |
| Dbindex          | 数据库id   |
| Ip       | 操作请求来源ip   |
| Option      | 操作命令   |
| Regexps | 用正则表达式匹配命令命中，操作次数 是key,value格式，key是匹配的正则，value是操作次数   |

例：
```
{
    "StartTime": "2017-12-22 17:05:40",
    "EndTime": "2017-12-22 17:12:40",
    "Regexp": "monitor;scheduler;queue_avalible_member;8000;queue;lock",
    "OperateSumCount": 111422,
    "Data": [
        {
            "Dbindex": "4",
            "Ip": "",
            "Option": "zincrby",
            "TotalCount": "1227",
            "Regexps": {}
        },
        {
            "Dbindex": "4",
            "Ip": "",
            "Option": "time",
            "TotalCount": "2",
            "Regexps": {}
        },
        {
            "Dbindex": "13",
            "Ip": "",
            "Option": "psubscribe",
            "TotalCount": "1",
            "Regexps": {}
        },
        {
            "Dbindex": "2",
            "Ip": "",
            "Option": "get",
            "TotalCount": "11",
            "Regexps": {}
        },
        {
            "Dbindex": "4",
            "Ip": "",
            "Option": "hset",
            "TotalCount": "2486",
            "Regexps": {
                "8000": 818,
                "queue": 1227,
                "queue_avalible_member": 1227
            }
        },
        {
            "Dbindex": "4",
            "Ip": "",
            "Option": "select",
            "TotalCount": "465",
            "Regexps": {}
        },
        {
            "Dbindex": "4",
            "Ip": "",
            "Option": "hgetall",
            "TotalCount": "63919",
            "Regexps": {}
        },
        {
            "Dbindex": "13",
            "Ip": "",
            "Option": "srem",
            "TotalCount": "2",
            "Regexps": {}
        },
        {
            "Dbindex": "8",
            "Ip": "",
            "Option": "get",
            "TotalCount": "6",
            "Regexps": {
                "lock": 6
            }
        },
        {
            "Dbindex": "0",
            "Ip": "",
            "Option": "ping",
            "TotalCount": "1511",
            "Regexps": {}
        },
        {
            "Dbindex": "0",
            "Ip": "",
            "Option": "zrem",
            "TotalCount": "450",
            "Regexps": {}
        },
        {
            "Dbindex": "4",
            "Ip": "",
            "Option": "lpush",
            "TotalCount": "9",
            "Regexps": {}
        },
        {
            "Dbindex": "4",
            "Ip": "",
            "Option": "get",
            "TotalCount": "42",
            "Regexps": {
                "lock": 7
            }
        }
    ]
}
```

