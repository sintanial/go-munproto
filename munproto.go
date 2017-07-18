package munproto

import (
	"net"
	"bufio"
	"fmt"
	"log"
)

var registeredProtos = make(map[string]func(*bufio.Reader) (bool, error))

// register new protocol
func RegisterProto(proto string, resolver func(*bufio.Reader) (bool, error)) {
	registeredProtos[proto] = resolver
}

// register predefined protocols
func RegisterProtoDefault() {
	RegisterProto("socks5", func(r *bufio.Reader) (bool, error) {
		data, err := r.Peek(1)
		if err != nil {
			return false, err
		}

		return data[0] == byte(5), nil
	})

	RegisterProto("https", func(r *bufio.Reader) (bool, error) {
		data, err := r.Peek(1)
		if err != nil {
			return false, err
		}

		return data[0] == byte(22), nil
	})

	RegisterProto("http", func(r *bufio.Reader) (bool, error) {
		return true, nil
	})
}

type listener struct {
	l      net.Listener
	proto  string
	connCh chan net.Conn
}

func (self *listener) Accept() (net.Conn, error) {
	return <-self.connCh, nil
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
	}
}

type Dispatcher struct {
	listeners map[string]*listener
	lorder    []string
	netl      net.Listener

	Logger *log.Logger
}

func NewDispatcher(l net.Listener) *Dispatcher {
	return &Dispatcher{
		listeners: make(map[string]*listener),
		netl:      l,
	}
}

// create listener for specific proto, which can use in http.Server.Serve(net.Listener) and etc..
// if used RegisterProtoDefault, the sequence of calls is very important, because "http" proto used as default and therefore
// other protocols should be called before "http"
func (self *Dispatcher) Listener(proto string) net.Listener {
	if _, ok := registeredProtos[proto]; !ok {
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
			if ne, ok := err.(net.Error); ok {
				if self.Logger != nil {
					self.Logger.Println("munproto: " + ne.Error())
				}

				continue
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

		resolver := registeredProtos[ls.proto]
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
