package main

import (
	"flag"
	"io"
	"log"
	"net"
	"os"
	"sync"
	protocol "tun/internal"
)

var tunnelFlag = flag.String("tunnel", "", "Tunnel address")

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
	if *tunnelFlag == "" {
		flag.Usage()
		os.Exit(1)
	}

	tunnel, err := listenTCP(*tunnelFlag)
	if err != nil {
		log.Fatalf("couldn't start tunnel: %v\n", err)
	}

	agent, err := tunnel.AcceptTCP()
	if err != nil {
		log.Fatalf("couldn't accept agent: %v\n", err)
	} else {
		log.Print("agent connected")
	}

	for {
		client, err := tunnel.AcceptTCP()
		if err != nil {
			log.Printf("error accepting connection: %v\n", err)
			continue
		}

		var wg sync.WaitGroup
		ingress := make(chan []byte)
		egress := make(chan []byte)

		wg.Add(1)
		go func() {
			defer func() {
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
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()

			for buffer := range ingress {
				message := protocol.Message{
					Kind:    protocol.Data,
					Payload: buffer,
				}

				message.Write(agent)
			}

			message := protocol.Message{
				Kind: protocol.EndOfStream,
			}

			message.Write(agent)
		}()

		wg.Add(1)
		go func() {
			defer func() {
				close(egress)
				wg.Done()
			}()

			for {
				message, err := protocol.ReadMessage(agent)
				if err != nil {
					log.Printf("couldn't read from agent: %v\n", err)
					return
				}

				if message.Kind == protocol.Data {
					egress <- message.Payload
				} else if message.Kind == protocol.EndOfStream {
					return
				}
			}

		}()

		wg.Add(1)
		go func() {
			defer func() {
				client.CloseWrite()
				wg.Done()
			}()

			for message := range egress {
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
		}()

		wg.Wait()
	}
}
