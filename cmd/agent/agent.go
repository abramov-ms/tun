package agent

import (
	"flag"
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

	service, err := dialTCP(*serviceFlag)
	if err != nil {
		log.Fatalf("couldn't connect to service: %v\n", err)
	} else {
		defer service.Close()
	}

	tunnel, err := dialTCP(*tunnelFlag)
	if err != nil {
		log.Fatalf("couldn't connec to tunnel: %v\n", err)
	} else {
		defer tunnel.Close()
	}

	ingress := make(chan []byte)
	egress := make(chan []byte)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			buffer := make([]byte, 1024)
			bytes, err := tunnel.Read(buffer)
			if err != nil {
				log.Print("error reading from client: %v\n", err)
				return
			}

			ingress <- buffer[:bytes]
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			message := <-ingress

			done := 0
			for done < len(message) {
				bytes, err := service.Write(message[done:])
				if err != nil {
					log.Print("error sending to service: %v\n", err)
					return
				}

				done += bytes
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			buffer := make([]byte, 1024)
			bytes, err := service.Read(buffer)
			if err != nil {
				log.Print("error reading from service: %v\n", err)
				return
			}

			egress <- buffer[:bytes]
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			message := <-egress

			done := 0
			for done < len(message) {
				bytes, err := tunnel.Write(message[done:])
				if err != nil {
					log.Print("error sending to tunnel: %v\n", err)
					return
				}

				done += bytes
			}
		}
	}()

	wg.Wait()
	os.Exit(1)
}
