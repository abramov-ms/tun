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

var (
	serviceFlag = flag.String("service", "", "Service address")
	tunnelFlag  = flag.String("tunnel", "", "Tunnel address")
)

func dialTCP(addrString string) (*net.TCPConn, error) {
	addr, err := net.ResolveTCPAddr("tcp", addrString)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func main() {
	flag.Parse()
	if *serviceFlag == "" || *tunnelFlag == "" {
		flag.Usage()
		os.Exit(1)
	}

	tunnel, err := dialTCP(*tunnelFlag)
	if err != nil {
		log.Fatalf("couldn't connec to tunnel: %v\n", err)
	}

	for {
		service, err := dialTCP(*serviceFlag)
		if err != nil {
			log.Fatalf("couldn't connect to service: %v\n", err)
		}

		ingress := make(chan []byte)
		egress := make(chan []byte)
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer func() {
				close(ingress)
				wg.Done()
			}()

			for {
				message, err := protocol.ReadMessage(tunnel)
				if err != nil {
					log.Printf("error reading from tunnel: %v\n", err)
					return
				}

				if message.Kind == protocol.Data {
					ingress <- message.Payload
				} else if message.Kind == protocol.EndOfStream {
					return
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer func() {
				service.CloseWrite()
				wg.Done()
			}()

			for buffer := range ingress {
				done := 0
				for done < len(buffer) {
					bytes, err := service.Write(buffer[done:])
					if err != nil {
						log.Printf("error sending to service: %v\n", err)
						return
					}

					done += bytes
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer func() {
				close(egress)
				wg.Done()
			}()

			for {
				buffer := make([]byte, 1024)
				bytes, err := service.Read(buffer)
				if err != nil {
					if err != io.EOF {
						log.Printf("error reading from service: %v\n", err)
					}

					return
				}

				egress <- buffer[:bytes]
			}
		}()

		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()
			}()

			for buffer := range egress {
				message := protocol.Message{
					Kind:    protocol.Data,
					Payload: buffer,
				}

				if err := message.Write(tunnel); err != nil {
					log.Printf("error writing to tunnel: %v\n", err)
					return
				}
			}

			message := protocol.Message{
				Kind: protocol.EndOfStream,
			}

			if err := message.Write(tunnel); err != nil {
				log.Printf("error writing to tunnel: %v\n", err)
			}
		}()

		wg.Wait()
	}
}
