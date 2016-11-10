// tcpsite.go - tiny sites that suck less than web does.
//
// To the extent possible under law, Ivan Markin waived all copyright
// and related or neighboring rights to this module of tcpsite, using the creative
// commons "cc0" public domain dedication. See LICENSE or
// <http://creativecommons.org/publicdomain/zero/1.0/> for full details.

package main

import(
	"log"
	"os"
	"net"
	"io"
	"flag"
	"strconv"
	"sync"
	"encoding/json"
	"path/filepath"

	"github.com/nogoegst/bulb"
)

func ServeTCPFile(l net.Listener, filename string) (error) {
	for {
		conn, err := l.Accept()
		if err !=nil {
			log.Printf("%v", err)
		}
		go func() {
			f, err := os.Open(filename)
			if err != nil {
				log.Printf("%v", err)
				return
			}
			defer f.Close()
			n, err := io.Copy(conn, f)
			log.Printf("Written %d bytes", n)
			if err != nil {
				log.Printf("%v", err)
				return
			}
			conn.Close()
		}()
	}
}

func ServeDirectory(l net.Listener, m map[string]uint16) {
	dir := make(map[string]uint16)
	for filename, port := range m {
		_, fname := filepath.Split(filename)
		dir[fname] = port
	}
	jsonString, _ := json.Marshal(dir)
	for {
		conn, err := l.Accept()
		if err !=nil {
			log.Printf("%v", err)
		}
		go func() {
			_, err := conn.Write(jsonString)
			if err != nil {
				log.Printf("Unable to write directory data to socket: %v", err)
			}
			conn.Close()
		}()
	}
}

const directoryPort = 65535
const directoryFilename = "[[directory]]"

func main() {
	var debugFlag = flag.Bool("debug", false,
		"Show what's happening")
	var control = flag.String("control-addr", "default://",
		"Set Tor control address to be used")
	var controlPasswd = flag.String("control-passwd", "",
		"Set Tor control auth password")
	flag.Parse()
	debug := *debugFlag
	if len(flag.Args()) != 1 {
		log.Fatalf("Please specify path to a file")
	}
	filePath := flag.Args()[0]

	// Connect to a running tor instance
	c, err := bulb.DialURL(*control)
	if err != nil {
		log.Fatalf("Failed to connect to control socket: %v", err)
	}
	defer c.Close()

	// See what's really going on under the hood
	c.Debug(debug)

	// Authenticate with the control port
	if err := c.Authenticate(*controlPasswd); err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	fileMap := make(map[string]uint16)

	fileMap[directoryFilename] = directoryPort
	fileMap[filePath] = 1

	portMap := make(map[uint16]string)

	var wg sync.WaitGroup

	for filename, port := range fileMap {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			log.Printf("%v", err)
			continue
		}
		defer l.Close()
		tAddr, ok := l.Addr().(*net.TCPAddr)
		if !ok {
			l.Close()
			log.Printf("failed to extract local port")
			continue
		}

		portMap[port] = strconv.FormatUint((uint64)(tAddr.Port), 10)

		wg.Add(1)
		if filename == directoryFilename {
			delete(fileMap, directoryFilename)
			go ServeDirectory(l, fileMap)
		} else {
			go ServeTCPFile(l, filename)
		}

	}
	var portSpec []bulb.OnionPortSpec
	for virtPort, target := range portMap {
		portSpec = append(portSpec, bulb.OnionPortSpec{VirtPort: virtPort, Target: target})
	}
	oi, err := c.AddOnion(portSpec, nil, true)
	if err != nil {
		log.Fatalf("%v", err)
	}
	log.Printf("Running on %s", oi.OnionID)
	wg.Wait()
}
