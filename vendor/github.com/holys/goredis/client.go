package goredis

import (
	"container/list"
	"io"
	"net"
	"strings"
	"sync"
	"time"
	"log"
)

type PoolConn struct {
	*Conn
	c *Client
}

func (c *PoolConn) Close() {
	if c.Conn.isClosed() {
		return
	}
	c.c.put(c.Conn)
}

// force close inner connection and not put it into pool
func (c *PoolConn) Finalize() {
	c.Conn.Close()
}

type Client struct {
	sync.Mutex

	addr            string
	maxIdleConns    int
	readBufferSize  int
	writeBufferSize int
	password        string

	conns *list.List

	quit chan struct{}
	wg   sync.WaitGroup
	logger *log.Logger

	closed bool
}

func getProto(addr string) string {
	if strings.Contains(addr, "/") {
		return "unix"
	} else {
		return "tcp"
	}
}

func NewClient(addr string, password string,logger *log.Logger) *Client {
	c := new(Client)

	c.addr = addr
	c.maxIdleConns = 10
	c.readBufferSize = 20 * 1024 * 1024
	c.writeBufferSize = 1024 * 1024
	c.password = password

	c.conns = list.New()
	c.quit = make(chan struct{})
	c.closed = false
	c.wg.Add(1)
	go c.onCheck() //ping不通会关闭链接，导致报错
	c.logger = logger
	return c
}

func (c *Client) SetPassword(pass string) {
	c.password = pass
}

func (c *Client) SetReadBufferSize(s int) {
	c.readBufferSize = s
}

func (c *Client) SetWriteBufferSize(s int) {
	c.writeBufferSize = s
}

func (c *Client) SetMaxIdleConns(n int) {
	c.maxIdleConns = n
}

func (c *Client) Do(cmd string, args ...interface{}) (interface{}, error) {
	var co *Conn
	var err error
	var r interface{}

	for i := 0; i < 2; i++ {
		co, err = c.get()
		if err != nil {
			return nil, err
		}

		r, err = co.Do(cmd, args...)
		if err != nil {
			if e, ok := err.(*net.OpError); ok && strings.Contains(e.Error(), "use of closed network connection") {
				//send to a closed connection, try again
				//continue
				return nil,err
			}
			c.put(co)
			return nil, err
		}

		c.put(co)
		return r, nil
	}

	return nil, err
}

func (c *Client) Monitor(respChan chan interface{}, stopChan chan struct{},closeChan *chan int) error {
	var co *Conn
	var err error
	if c == nil || c.addr == ""{
		c.logger.Println("client 已关闭")
		return nil
	}
	c.closed = false
	co, err = c.newConn(c.addr, c.password)//改造，监控自己创建连接，连接断开后重新创建
	//co, err = c.get()//old code
	if err != nil {
		c.logger.Println("Monitor create conn error,",err)
		return err
	}

	if err := co.Send("MONITOR"); err != nil {
		return err
	}

	go func() {
		/*defer func() {
			c.put(co)
		}()*/
		defer co.Close()
		log.Println("start clint resp")
		for {
			select {
			case <- *closeChan:{
				//co.Close()
				stopChan <- struct{}{}
				log.Println("close m 0")
				break
			}
			default:{
				resp, err := co.Receive()
				if err != nil {
					if e, ok := err.(*net.OpError); ok && strings.Contains(e.Error(), "use of closed network connection 2") || err == io.EOF {
						//the server may has closed the connection
						log.Println("close m 1")
					}
					respChan <- err
				}
				if c == nil || c.closed {
					log.Println("close m 2")
				}
				defer func() {
					if err:=recover();err != nil{
						log.Println("sen respChan err:",err)
					}
				}()
				if _,ok := <- respChan;!ok{
					log.Println("respChan closed")
					stopChan <- struct{}{}
					log.Println("stop client resp goroutine")
					break
				}
				respChan <- resp
			}
			}
		}


	}()

	return nil
}

func (c *Client) Close() {
	c.Lock()
	defer c.Unlock()

	close(c.quit)
	c.wg.Wait()
	c.closed = true
	for c.conns.Len() > 0 {
		e := c.conns.Front()
		co := e.Value.(*Conn)
		c.conns.Remove(e)
		c.logger.Println("close3")
		co.Close()
	}
}

func (c *Client) Get() (*PoolConn, error) {
	co, err := c.get()
	if err != nil {
		return nil, err
	}

	return &PoolConn{co, c}, err
}

func (c *Client) get() (co *Conn, err error) {
	c.Lock()
	if c.conns.Len() == 0 {
		c.Unlock()
		co, err = c.newConn(c.addr, c.password)
		c.logger.Println("new Con")
	} else {
		e := c.conns.Front()
		co = e.Value.(*Conn)
		c.conns.Remove(e)
		c.logger.Println("use Con")
		c.Unlock()
	}
	if co.closed == 1 {
		c.logger.Println(" Con close use new con")
		co, err = c.newConn(c.addr, c.password)
	}
	return
}

func (c *Client) put(conn *Conn) {
	c.Lock()
	defer c.Unlock()

	for c.conns.Len() >= c.maxIdleConns {
		// remove back
		e := c.conns.Back()
		co := e.Value.(*Conn)
		c.conns.Remove(e)
		c.logger.Println("close2")
		co.Close()
	}

	c.conns.PushFront(conn)
}

func (c *Client) getIdle() *Conn {
	c.Lock()
	defer c.Unlock()

	if c.conns.Len() == 0 {
		return nil
	} else {
		e := c.conns.Back()
		co := e.Value.(*Conn)
		c.conns.Remove(e)
		return co
	}
}

func (c *Client) checkIdle() {
	co := c.getIdle()
	if co == nil {
		return
	}

	_, err := co.Do("PING")
	if err != nil {
		c.logger.Println("close1")
		co.Close()

	} else {
		c.put(co)
	}
}

func (c *Client) onCheck() {
	t := time.NewTicker(10 * time.Second)

	defer func() {
		t.Stop()
		c.wg.Done()
	}()

	for {
		select {
		case <-t.C:
			c.checkIdle()
		case <-c.quit:
			return
		}
	}
}
