// mux
// MIT License Copyright(c) 2019 Hiroshi Shimamoto
// vim:set sw=4 sts=4:
package main

import (
    "fmt"
    "io"
    "log"
    "net"
    "os"
    "strings"

    "github.com/BurntSushi/toml"
    "github.com/hshimamoto/go-iorelay"
)

type entry struct {
    Index int
    Name string
    Connect string
}

type tomldata struct {
    Type string
    Listen string
    Server string
    Entry []entry
}

type config struct {
    path string
    prev, data *tomldata
    //
    l net.Listener
}

func (c *config)Load() {
    td := &tomldata{
	Type: "client",
    }
    if _, err := toml.DecodeFile(c.path, td); err != nil {
	log.Printf("config %s load error: %v\n", c.path, err)
	return
    }
    c.prev = c.data
    c.data = td; // replace
    log.Println(td)
}

func (c *config)SetupListener() error {
    addr := c.data.Listen
    a := strings.SplitN(addr, ":", 2)
    proto := "tcp"
    if a[0] == "unix" {
	proto = "unix"
	addr = a[1]
    }
    l, err := net.Listen(proto, addr)
    if err != nil {
	log.Printf("Listen %s: %v\n", addr, err)
	return err
    }
    c.l = l
    return nil
}

func dialto(addr string) (net.Conn, error) {
    a := strings.SplitN(addr, ":", 2)
    proto := "tcp"
    if a[0] == "unix" {
	proto = "unix"
	addr = a[1]
    }
    return net.Dial(proto, addr)
}

func readline(reader io.Reader) string {
    line := ""
    char := make([]byte, 1)
    for {
	n, _ := reader.Read(char)
	if n == 0 {
	    return ""
	}
	if char[0] == '\n' {
	    return line
	}
	line += string(char)
    }
}

func (c *config)Connect(conn net.Conn, name string) {
    defer conn.Close()

    log.Printf("connecting to %s\n", name)
    for _, entry := range c.data.Entry {
	if entry.Name == name {
	    log.Printf("try to connect %s\n", entry.Connect)
	    fwd, err := dialto(entry.Connect)
	    if err != nil {
		log.Printf("failed %v\n", err)
		return
	    }
	    defer fwd.Close()
	    log.Println("connected")
	    iorelay.Relay(conn, fwd)
	    log.Println("session done")
	    return
	}
    }
    log.Printf("unknwon entry %s\n", name)
}

func (c *config)HandleServer(conn net.Conn) {
    // update
    c.Load()
    // create entry list
    list := make([]string, len(c.data.Entry))
    for i, entry := range c.data.Entry {
	list[i] = entry.Name
    }
    conn.Write([]byte(strings.Join(list, " ") + "\n"))
    // get name
    name := readline(conn)
    if name == "" {
	return
    }
    // this should be an entry name
    go c.Connect(conn, name)
}

func (c *config)DialToServer() (net.Conn, error) {
    return dialto(c.data.Server)
}

func (c *config)HandleClient(conn net.Conn) {
    server, err := c.DialToServer()
    if err != nil {
	log.Printf("DialToServer %s: %v\n", c.data.Server, err)
	conn.Close()
	return
    }

    // run in main thread
    line := readline(server)
    fmt.Printf("select server: %s\n", line)
    entries := strings.Split(line, " ")
    //
    ok := func(n string) bool {
	for _, e := range entries {
	    if n == e {
		return true
	    }
	}
	return false
    }
    name := ""
    for {
	name = readline(os.Stdin)
	if ok(name) {
	    break
	}
	fmt.Printf("no %s in %s\nselect again\n", name, entries)
    }
    server.Write([]byte(name + "\n"))

    go func() {
	defer conn.Close()
	defer server.Close()

	log.Println("session start")
	iorelay.Relay(conn, server)
	log.Println("session done")
    }()
}

func (c *config)Handler(conn net.Conn) {
    switch c.data.Type {
    case "server": c.HandleServer(conn)
    case "client": c.HandleClient(conn)
    }
}

func (c *config)Serv() {
    c.SetupListener()
    for {
	conn, err := c.l.Accept()
	if err != nil {
	    // something wrong
	    continue
	}
	c.Handler(conn)
    }
}

func main() {
    if len(os.Args) < 2 {
	log.Println("mux <config file>")
	return
    }
    config := &config{
	path: os.Args[1],
	data: nil,
	prev: nil,
    }
    config.Load()
    config.Serv()
}
