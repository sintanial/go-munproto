package munproto

import (
	"net"
	"bufio"
	"fmt"
	"log"
	"sync"
	"strings"
)

var defaultProtos = map[string]func(*bufio.Reader) (bool, error){
	"socks5": IsSOCKS5,
	"socks4": IsSOCKS4,
	"https":  IsHTTPS,
	"http":   IsHTTP,
}

func IsSOCKS5(r *bufio.Reader) (bool, error) {
	data, err := r.Peek(1)
	if err != nil {
		return false, err
	}

	return data[0] == byte(5), nil
}

func IsSOCKS4(r *bufio.Reader) (bool, error) {
	data, err := r.Peek(1)
	if err != nil {
		return false, err
	}

	return data[0] == byte(4), nil
}

func IsHTTPS(r *bufio.Reader) (bool, error) {
	data, err := r.Peek(1)
	if err != nil {
		return false, err
	}

	return data[0] == byte(22), nil
}

func IsHTTP(r *bufio.Reader) (bool, error) {
	data, err := r.Peek(7)
	if err != nil {
		return false, err
	}

	method := strings.ToUpper(strings.Split(string(data), " ")[0])
	if method == "GET" || method == "HEAD" || method == "POST" || method == "PUT" || method == "DELETE" || method == "CONNECT" || method == "OPTIONS" || method == "TRACE" || method == "PATCH" {
		return true, nil
	}

	return false, nil
}

type listener struct {
	l      net.Listener
	proto  string
	connCh chan net.Conn
	errCh  chan error
}

func (self *listener) Accept() (net.Conn, error) {
	select {
	case conn := <-self.connCh:
		return conn, nil
	case err := <-self.errCh:
		return nil, err
	}
}

func (self *listener) Addr() net.Addr {
	return self.l.Addr()
}

func (self *listener) Close() error {
	return self.l.Close()
}

func newListener(l net.Listener, proto string) *listener {
	return &listener{
		l:      l,
		proto:  proto,
		connCh: make(chan net.Conn),
		errCh:  make(chan error),
	}
}

type Dispatcher struct {
	mu     sync.RWMutex
	protos map[string]func(*bufio.Reader) (bool, error)

	listeners map[string]*listener
	lorder    []string
	netl      net.Listener

	Logger *log.Logger
}

func New(l net.Listener) *Dispatcher {
	return &Dispatcher{
		protos:    make(map[string]func(*bufio.Reader) (bool, error)),
		listeners: make(map[string]*listener),
		netl:      l,
	}
}

func NewDefault(l net.Listener) *Dispatcher {
	d := New(l)
	for name, detectfn := range defaultProtos {
		d.AddProto(name, detectfn)
	}
	return d
}

func (self *Dispatcher) AddProto(name string, detectfn func(*bufio.Reader) (bool, error)) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.protos[name] = detectfn
}

// create listener for specific proto, which can use in http.Server.Serve(net.Listener) and etc..
// if used RegisterProtoDefault, the sequence of calls is very important, because "http" proto used as default and therefore
// other protocols should be called before "http"
func (self *Dispatcher) Listener(proto string) net.Listener {
	if _, ok := self.protos[proto]; !ok {
		panic(fmt.Sprintf("undefined proto: %s", proto))
	}

	self.lorder = append(self.lorder, proto)

	l := newListener(self.netl, proto)
	self.listeners[proto] = l
	return l
}

// listen interface, and rotate between different registered proto
func (self *Dispatcher) Listen() error {
	for {
		conn, err := self.netl.Accept()
		if err != nil {
			if nerr, ok := err.(net.Error); ok {
				if self.Logger != nil {
					self.Logger.Println("munproto: " + nerr.Error())
				}
				continue
			}

			for _, l := range self.listeners {
				l.errCh <- err
			}
			return err
		}

		go self.dispatch(conn)
	}
}

func (self *Dispatcher) dispatch(conn net.Conn) {
	bufconn := newBufConn(conn)

	for _, proto := range self.lorder {
		ls := self.listeners[proto]

		resolver := defaultProtos[ls.proto]
		isSuitableProto, err := resolver(bufconn.r)
		if err != nil {
			if self.Logger != nil {
				self.Logger.Println("munproto: " + err.Error())
			}
			return
		}

		if isSuitableProto {
			ls.connCh <- bufconn
			return
		}
	}

	bufconn.Close()
}

// todo: добавить пул
type bufConn struct {
	r *bufio.Reader
	net.Conn
}

func (self *bufConn) Read(b []byte) (n int, err error) {
	return self.r.Read(b)
}

func newBufConn(c net.Conn) *bufConn {
	return &bufConn{bufio.NewReader(c), c}
}
