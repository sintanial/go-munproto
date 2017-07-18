Support multiple protocols such as http, https, socks5, etc..., on one port.

# Install 
`go get github.com/sintanial/go-munproto`

## Docs 
https://godoc.org/github.com/sintanial/go-munproto

## Usage 

```golang
package main

import (
	"net"
	"github.com/sintanial/go-munproto"
	"fmt"
	"log"
	"os"
)

func main() {
	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}

	munproto.RegisterProtoDefault()

	dispatcher := munproto.NewDispatcher(l)
	dispatcher.Logger = log.New(os.Stdout, "[munproto debug] ", log.LstdFlags)

	// return interface net.Listener
	// sequence of calls very important, for more detailes see Dispatcher.Listener doc 
	socks5Listener := dispatcher.Listener("socks5")
	httpsListener := dispatcher.Listener("https")
	httpListener := dispatcher.Listener("http")

    // if you use github.com/armon/go-socks5 for create SOCKS5 library,
    // you can create socks5.Server{} and pass socks5Listener to socks5.Server.Serve()
	go serve("socks5", socks5Listener)
	
	// you can create http.Server{} and pass httpListener to http.Server.Serve()
	go serve("http", httpListener)
	go serve("https", httpsListener)

	if err := dispatcher.Listen(); err != nil {
		panic(err)
	}
}

func serve(proto string, l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}

		fmt.Println(fmt.Sprintf("handle %s conn", proto), conn.RemoteAddr())
		conn.Close()
	}
}
```

### TODO
Write tests