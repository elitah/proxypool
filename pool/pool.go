package pool

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Map interface {
	Delete(interface{})
	Load(interface{}) (interface{}, bool)
	LoadOrStore(interface{}, interface{}) (interface{}, bool)
	Range(func(interface{}, interface{}) bool)
	Store(interface{}, interface{})
}

type ProxyManager interface {
	Map
	sort.Interface

	IsExist(interface{}) bool
	CloseBad(int64)
	GetSortList() []*proxyNode
}

type proxyChecker interface {
	IsExit() bool

	GetAddress() (string, string)

	CheckConn(net.Conn) bool

	ShowError(error)
}

type proxyConn struct {
	net.Conn

	node *proxyNode
}

func (this *proxyConn) Close() error {
	if nil != this.node {
		atomic.AddInt64(&this.node.count, -1)
	}
	return this.Conn.Close()
}

type proxyNode struct {
	flags uint32

	exit uint32

	total   int64
	okcnt   int64
	failcnt int64

	timeout int64

	usability int64

	last int64

	count int64

	fn func(string, string) (net.Conn, error)
}

func (this *proxyNode) Close() {
	atomic.CompareAndSwapUint32(&this.exit, 0x0, 0x1)
}

func (this *proxyNode) StartCheck(checker proxyChecker) {
	if atomic.CompareAndSwapUint32(&this.flags, 0x0, 0x1) {
		go func() {
			var now time.Time

			var network, address string

			defer atomic.StoreUint32(&this.flags, 0x0)

			for !checker.IsExit() && 0x0 == atomic.LoadUint32(&this.exit) {
				//
				now = time.Now()
				//
				network, address = checker.GetAddress()
				// 总数+1
				atomic.AddInt64(&this.total, 1)
				//
				if conn, err := this.fn(network, address); nil == err {
					if checker.CheckConn(conn) {
						// 成功+1
						atomic.AddInt64(&this.okcnt, 1)
						// 复位失败
						atomic.StoreInt64(&this.failcnt, 0)
						// 更新超时
						atomic.StoreInt64(&this.timeout, time.Since(now).Nanoseconds()/1000000)
					} else {
						// 失败+1
						atomic.AddInt64(&this.failcnt, 1)
					}
					//
					conn.Close()
				} else {
					// 失败+1
					atomic.AddInt64(&this.failcnt, 1)
					//
					checker.ShowError(err)
				}

				// 可用性更新
				atomic.StoreInt64(
					&this.usability,
					int64((float64(atomic.LoadInt64(&this.okcnt))/float64(atomic.LoadInt64(&this.total)))*100.0),
				)

				// 更新最后时间
				atomic.StoreInt64(&this.last, now.UnixNano())

				// 根据错误次数睡眠
				time.Sleep(this.slow(int(atomic.LoadInt64(&this.failcnt)/3)) * time.Second)
			}
		}()
	}
}

func (this *proxyNode) Compare(x *proxyNode, method string) bool {
	switch method {
	case "total":
		return atomic.LoadInt64(&x.total) < atomic.LoadInt64(&this.total)
	case "okcnt":
		return atomic.LoadInt64(&x.okcnt) < atomic.LoadInt64(&this.okcnt)
	case "failcnt":
		return atomic.LoadInt64(&x.failcnt) > atomic.LoadInt64(&this.failcnt)
	case "timeout":
		return atomic.LoadInt64(&x.timeout) > atomic.LoadInt64(&this.timeout)
	case "usability":
		return atomic.LoadInt64(&x.usability) < atomic.LoadInt64(&this.usability)
	case "last":
		return atomic.LoadInt64(&x.last) < atomic.LoadInt64(&this.last)
	case "count":
		return atomic.LoadInt64(&x.count) > atomic.LoadInt64(&this.count)
	}
	return false
}

func (this *proxyNode) Get(method string) int64 {
	switch method {
	case "total":
		return atomic.LoadInt64(&this.total)
	case "okcnt":
		return atomic.LoadInt64(&this.okcnt)
	case "failcnt":
		return atomic.LoadInt64(&this.failcnt)
	case "timeout":
		return atomic.LoadInt64(&this.timeout)
	case "usability":
		return atomic.LoadInt64(&this.usability)
	case "last":
		return atomic.LoadInt64(&this.last)
	case "count":
		return atomic.LoadInt64(&this.count)
	}
	return 0
}

func (this *proxyNode) Dial(network string, address string) (net.Conn, error) {
	if conn, err := this.fn(network, address); nil == err {
		atomic.AddInt64(&this.count, 1)
		return &proxyConn{conn, this}, nil
	} else {
		return nil, err
	}
}

func (this *proxyNode) String() string {
	return fmt.Sprintf(
		"%8d, %8d, %8d, %8d, %3d, %32d, %8d",
		atomic.LoadInt64(&this.total),
		atomic.LoadInt64(&this.okcnt),
		atomic.LoadInt64(&this.failcnt),
		atomic.LoadInt64(&this.timeout),
		atomic.LoadInt64(&this.usability),
		atomic.LoadInt64(&this.last),
		atomic.LoadInt64(&this.count),
	)
}

func (this *proxyNode) slow(ratio int) time.Duration {
	var result float64 = 3

	for i := 0; ratio > i; i++ {
		result *= 5 / 3
	}

	return time.Duration(result)
}

func (this *proxyNode) before(x *proxyNode) bool {
	return atomic.LoadInt64(&x.usability) < atomic.LoadInt64(&this.usability) &&
		atomic.LoadInt64(&x.last) < atomic.LoadInt64(&this.last)
}

type proxyManager struct {
	sync.Map

	idx uint64

	length uint64

	flags uint32

	lists []*proxyNode
}

func (this *proxyManager) Add(node *proxyNode) uint64 {
	idx := atomic.AddUint64(&this.idx, 1) - 1

	this.Store(idx, node)

	return idx
}

func (this *proxyManager) Store(key, value interface{}) {
	defer atomic.AddUint64(&this.length, 1)

	this.Map.Store(key, value)
}

func (this *proxyManager) Delete(key interface{}) {
	defer atomic.AddUint64(&this.length, ^uint64(0))

	this.Map.Delete(key)
}

func (this *proxyManager) IsExist(key interface{}) bool {
	//
	_, ok := this.Load(key)
	//
	return ok
}

func (this *proxyManager) CloseBad(t int64) {
	if 3 > t {
		t = 3
	}
	this.Range(func(key, value interface{}) bool {
		if node, ok := value.(*proxyNode); ok {
			if t < atomic.LoadInt64(&node.failcnt) {
				// 关闭
				node.Close()
				// 删除
				this.Delete(key)
			}
		}
		return true
	})
}

func (this *proxyManager) GetSortList() []*proxyNode {
	if atomic.CompareAndSwapUint32(&this.flags, 0x0, 0x1) {
		defer func() {
			this.lists = nil
		}()

		this.lists = make([]*proxyNode, 0, this.Len())

		this.Range(func(key, value interface{}) bool {
			if node, ok := value.(*proxyNode); ok {
				this.lists = append(this.lists, node)
			}
			return true
		})

		sort.Sort(this)

		atomic.StoreUint32(&this.flags, 0x0)

		return this.lists
	}

	return nil
}

func (this *proxyManager) Len() int {
	return int(atomic.LoadUint64(&this.length))
}

func (this *proxyManager) Swap(i, j int) {
	this.lists[i], this.lists[j] = this.lists[j], this.lists[i]
}

func (this *proxyManager) Less(i, j int) bool {
	return this.lists[i].before(this.lists[j])
}

func NewProxyManager() ProxyManager {
	return &proxyManager{}
}

func NewProxyNode(fn func(string, string) (net.Conn, error)) *proxyNode {
	if nil != fn {
		return &proxyNode{
			fn: fn,
		}
	}
	return nil
}
