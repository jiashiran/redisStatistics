# redisStatistics
redis日志统计，统计redis某些操作的次数

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
| options     | 必选   | 操作指令，多个用“,”区分                            | HGET,HSET           |
| ip          | 可选   | 请求来自哪些ip，多个用“,”区分，不配置统计所有来源              | 172.16.203.193      |
| index       | 可选   | 数据库索引，多个用“,”区分，不配置统计所有数据库                | 1,2,3,4,5,6,7,8     |
| regexp      | 可选   | 操作参数匹配的正则表达式，不配置统计所有                     | provider            |
| saveToIndex | 可选   | 统计结果保存的redis数据库，不配置默认为0                  | 2                   |
| httpPort    | 可选   | 控制打开或关闭监控的rest接口的端口，不配置默认为8080           |                     |
| logFlag     | 可选   | 日志模式，可配置info,debug,file，不配置默认为info,file模式日志输出到文件 |                     |



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

监控会将结果以json格式存入指定数据库中，key为“redis_statistics”，数据每分钟同步一次，停止监控空后数据有效期为1小时

例：

```
{
    "StartTime": "2017-04-25 17:06:30",            //统计开始时间
    "EndTime": "2017-04-25 17:07:30",              //统计截止时间
    "Regexp": "provider",                          //需要匹配正则表达式
    "Data": [
        {
            "Dbindex": "3",        //数据库id
            "Ip": "",              //操作请求来源ip
            "Option": "hset",      //操作命令
            "TotalCount": "2",     //操作命令总次数
            "Count": "2"           //用正则表达式匹配命令命中，操作次数
        },
        {
            "Dbindex": "5",
            "Ip": "",
            "Option": "hset",
            "TotalCount": "6",
            "Count": "0"
        },
        {
            "Dbindex": "8",
            "Ip": "",
            "Option": "hset",
            "TotalCount": "26",
            "Count": "22"
        }
    ]
}
```

