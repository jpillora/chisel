package chclient

import (
	"fmt"
	"io"
	"net"

	"github.com/jpillora/chisel/share"
	"time"
	"github.com/silenceper/pool"
)

const (
	PoolInitCap = 15
	PoolMaxCap = 50
)

type tcpProxy struct {
	*chshare.Logger
	client *Client
	id     int
	count  int
	remote *chshare.Remote
}

var pm map[string]pool.Pool

func newTCPProxy(c *Client, index int, remote *chshare.Remote) *tcpProxy {
	id := index + 1
	return &tcpProxy{
		Logger: c.Logger.Fork("tunnel#%d %s", id, remote),
		client: c,
		id:     id,
		remote: remote,
	}
}

func (p *tcpProxy) start() error {
	l, err := net.Listen("tcp4", p.remote.LocalHost+":"+p.remote.LocalPort)
	if err != nil {
		return fmt.Errorf("%s: %s", p.Logger.Prefix(), err)
	}
	go p.listen(l)
	return nil
}

func (p *tcpProxy) listen(l net.Listener) {
	p.Infof("Listening")
	for {
		src, err := l.Accept()
		if err != nil {
			p.Infof("Accept error: %s", err)
			return
		}
		go p.accept(src)
	}
}

func (p *tcpProxy) accept(src io.ReadWriteCloser) {
	p.count++
	cid := p.count
	l := p.Fork("conn#%d", cid)
	l.Debugf("Open")
	if p.client.sshConn == nil {
		l.Debugf("No server connection")
		src.Close()
		return
	}
	fmt.Println("Now openStream with :", p.remote.Remote())

	//TODO get ioReadWriteClose from pool
	pm, err := p.checkOrIntPool()
	if err != nil {
		fmt.Println("Can not get pool")
	}
	v, err := pm.Get()

	if err != nil {
		v, _ = pm.Get()
	}
	if  v == nil {
		src.Close()
		fmt.Println("Can not connect remote")
		return
	}
	fmt.Println("Get conn from pool")
	dst := v.(io.ReadWriteCloser)
//	dst, err := chshare.OpenStream(p.client.sshConn, p.remote.Remote())
	if err != nil {
		l.Infof("Stream error: %s", err)
		src.Close()
		return
	}
	//then pipe
	s, r := chshare.Pipe(src, dst)
	l.Debugf("Close (sent %d received %d)", s, r)
}

func (p *tcpProxy) fillPool(pm pool.Pool) {
	if pm == nil {
		return
	}
	fmt.Println("Before fill pool, the pool size->", pm.Len())
	for i := pm.Len(); i <= PoolInitCap; i++ {
		if pm.Len() > PoolInitCap {
			break
		}
		if p.client.sshConn == nil {
			break
		}
		conn, err := chshare.OpenStream(p.client.sshConn, p.remote.Remote())
		if  err == nil {
			pm.Put(conn)
		} else {
			conn, err = chshare.OpenStream(p.client.sshConn, p.remote.Remote())
			if err == nil {
				pm.Put(conn)
			}
		}
	}
	fmt.Println("Now the pool size->", pm.Len())
}

func (p *tcpProxy) checkOrIntPool() (pool.Pool, error){
	remote := p.remote.Remote()
	if pm[remote] == nil {
		factory := func() (interface{}, error) {
			conn, err := chshare.OpenStream(p.client.sshConn, remote)
			return conn, err
		}
		close := func(v interface{}) error { return v.(io.ReadWriteCloser).Close() }
		poolConfig := &pool.PoolConfig{
			InitialCap: PoolInitCap,
			MaxCap:     PoolMaxCap,
			Factory:    factory,
			Close:      close,
			IdleTimeout: 60 * time.Second,
		}
		p, err := pool.NewChannelPool(poolConfig)
		if err != nil {
			fmt.Println("err=", err)
			return nil, err
		}
		if len(pm) == 0 {
			pm = make(map[string]pool.Pool)
		}
		pm[remote] = p
	} else {
		fmt.Println("Got factory from pool ", remote, pm[remote], "The len of the pool->", pm[remote].Len())
	}
	go p.fillPool(pm[remote])
	return pm[remote], nil
}