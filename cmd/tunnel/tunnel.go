package tunnel

import (
	"flag"
	"log"
	"net"
	"os"
	"sync"
)

var (
	tunnelFlag = flag.String("tunnel", "", "Tunnel address")
	agentFlag  = flag.String("agent", "", "Agent address")
)

func listenTCP(addrString string) (*net.TCPListener, error) {
	addr, err := net.ResolveTCPAddr("tcp", addrString)
	if err != nil {
		return nil, err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return listener, nil
}

func main() {
	flag.Parse()
	if *tunnelFlag == "" || *agentFlag == " " {
		flag.Usage()
		os.Exit(1)
	}

	tunnel, err := listenTCP(*tunnelFlag)
	if err != nil {
		log.Fatalf("couldn't start tunnel: %v\n", err)
	}

	agentAddr, err := net.ResolveTCPAddr("tcp", *agentFlag)
	if err != nil {
		log.Fatalf("couldn't resolve agent address: %v\n", err)
	}

	var agent *net.TCPConn
	agentConnected := make(chan struct{})
	waitForAgent := func() {
		_, _ = <-agentConnected
	}

	ingress := make(chan []byte)
	egress := make(chan []byte)

	for {
		conn, err := tunnel.AcceptTCP()
		if err != nil {
			log.Printf("error accepting connection: %v\n", err)
			continue
		}

		if conn.RemoteAddr().String() == agentAddr.String() {
			agent = conn
			close(agentConnected)

			go func() {
				for {
					buffer := make([]byte, 1024)
					bytes, err := agent.Read(buffer[:])
					if err != nil {
						log.Printf("couldn't read from agent: %v\n", err)
						return
					}

					egress <- buffer[:bytes]
				}
			}()

			log.Println("connected to agent")
			continue
		}

		var wg sync.WaitGroup

		wg.Add(1)
		go func(client *net.TCPConn) {
			waitForAgent()
			defer wg.Done()

			for {
				buffer := make([]byte, 1024)
				bytes, err := client.Read(buffer[:])
				if err != nil {
					log.Printf("couldn't read from client: %v\n", err)
					return
				}

				ingress <- buffer[:bytes]
				if bytes == 0 {
					return
				}
			}
		}(conn)

		wg.Add(1)
		go func() {
			waitForAgent()
			defer wg.Done()

			for {
				message := <-ingress
				if len(message) == 0 {
					return
				}

				done := 0
				for done < len(message) {
					bytes, err := agent.Write(message[done:])
					if err != nil {
						log.Printf("error writing to agent: %v\n", err)
						return
					}

					done += bytes
				}
			}
		}()

		wg.Add(1)
		go func(client *net.TCPConn) {
			waitForAgent()
			defer wg.Done()

			for {
				message := <-egress

				done := 0
				for done < len(message) {
					bytes, err := client.Write(message[done:])
					if err != nil {
						log.Printf("error writing to client: %v\n", err)
						return
					}

					done += bytes
				}
			}
		}(conn)
	}
}
