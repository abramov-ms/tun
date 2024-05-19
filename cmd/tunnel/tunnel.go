package main

import (
	"flag"
	"io"
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

	for {
		var agent *net.TCPConn
		agentConnected := make(chan struct{})
		waitForAgent := func() {
			_, _ = <-agentConnected
		}

		var wg sync.WaitGroup
		ingress := make(chan []byte)
		egress := make(chan []byte)
		clientConnected := false

		for {
			conn, err := tunnel.AcceptTCP()
			if err != nil {
				log.Printf("error accepting connection: %v\n", err)
				continue
			}

			if conn.RemoteAddr().String() == agentAddr.String() {
				agent = conn
				close(agentConnected)

				wg.Add(1)
				go func() {
					defer func() {
						agent.CloseRead()
						close(egress)
						wg.Done()
					}()

					for {
						buffer := make([]byte, 1024)
						bytes, err := agent.Read(buffer[:])
						if err != nil {
							if err != io.EOF {
								log.Printf("couldn't read from agent: %v\n", err)
							}

							return
						}

						egress <- buffer[:bytes]
					}
				}()

				log.Println("connected to agent")
				continue
			}

			if clientConnected {
				conn.Close()
				log.Print("cannot handle more than one client at once")
				continue
			} else {
				clientConnected = true
			}

			wg.Add(1)
			go func(client *net.TCPConn) {
				waitForAgent()
				defer func() {
					client.CloseRead()
					close(ingress)
					wg.Done()
				}()

				for {
					buffer := make([]byte, 1024)
					bytes, err := client.Read(buffer[:])
					if err != nil {
						if err != io.EOF {
							log.Printf("couldn't read from client: %v\n", err)
						}

						return
					}

					ingress <- buffer[:bytes]
				}
			}(conn)

			wg.Add(1)
			go func() {
				waitForAgent()
				defer func() {
					agent.CloseWrite()
					wg.Done()
				}()

				for {
					message, ok := <-ingress
					if !ok {
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
				defer func() {
					client.CloseWrite()
					wg.Done()
				}()

				for {
					message, ok := <-egress
					if !ok {
						return
					}

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

			if agent != nil {
				wg.Wait()
				break
			}
		}
	}
}
