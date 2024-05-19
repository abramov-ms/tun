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

	for {
		service, err := dialTCP(*serviceFlag)
		if err != nil {
			log.Fatalf("couldn't connect to service: %v\n", err)
		}

		tunnel, err := dialTCP(*tunnelFlag)
		if err != nil {
			log.Fatalf("couldn't connec to tunnel: %v\n", err)
		}

		ingress := make(chan []byte)
		egress := make(chan []byte)
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer func() {
				tunnel.CloseRead()
				close(ingress)
				wg.Done()
			}()

			for {
				buffer := make([]byte, 1024)
				bytes, err := tunnel.Read(buffer)
				if err != nil {
					if err != io.EOF {
						log.Printf("error reading from tunnel: %v\n", err)
					}

					return
				}

				ingress <- buffer[:bytes]
			}
		}()

		wg.Add(1)
		go func() {
			defer func() {
				service.CloseWrite()
				wg.Done()
			}()

			for {
				message, ok := <-ingress
				if !ok {
					return
				}

				done := 0
				for done < len(message) {
					bytes, err := service.Write(message[done:])
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
				service.CloseRead()
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
				tunnel.CloseWrite()
				wg.Done()
			}()

			for {
				message, ok := <-egress
				if !ok {
					return
				}

				done := 0
				for done < len(message) {
					bytes, err := tunnel.Write(message[done:])
					if err != nil {
						log.Printf("error sending to tunnel: %v\n", err)
						return
					}

					done += bytes
				}
			}
		}()

		wg.Wait()
	}
}
