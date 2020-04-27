package main

import (
	"flag"
	"log"
	"os"

	"github.com/kingzbauer/gmail-attachments/gmail"
)

var (
	c       = flag.String("c", "", "config file")
	subject = flag.String("s", "", "subject for service a/c to impersonate")
	q       = flag.String("q", "", "Gmail like query to filter across messages")
)

func chk(msg string, err error) {
	if err != nil {
		log.Fatalf("Error %s => %s", msg, err)
	}
}

func main() {
	flag.Parse()
	if len(*c) == 0 {
		flag.Usage()
		log.Fatal("c config file required")
	}

	if len(*subject) == 0 {
		flag.Usage()
		log.Fatal("s subject required")
	}

	if *q == "" {
		flag.Usage()
		log.Fatal("q query required")
	}

	f, err := os.Open(*c)
	chk("Open config file", err)
	srv, err := gmail.NewService(f, *subject)
	chk("Initialize service", err)
	srv.DefaultQ = "is:unread from:m-pesastatements@safaricom.co.ke"

	attachments, err := srv.ProcessPDFAttachments(true)
	if attachments != nil {
		attachments.Close()
		for _, at := range attachments {
			log.Printf("Original filename: %s", at.OriginalName)
			for _, header := range at.Headers {
				log.Printf("Name: %s, Value: %s", header.Name, header.Value)
			}
		}
	}
	log.Println(err)
}
