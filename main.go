// Go equivalent of the "DNS & BIND" book check-soa program.
// Created by Stephane Bortzmeyer.
package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"time"

	"github.com/miekg/dns"
)

const (
	// DefaultTimeout is default timeout many operation in this program will
	// use.
	DefaultTimeout time.Duration = 5 * time.Second
)

var (
	localm          *dns.Msg
	localc          *dns.Client
	conf            *dns.ClientConfig
	domain          string
	assembledDomain string
)

func init() {
	rand.Seed(time.Now().Unix())
}

func localQuery(qname string, qtype uint16, server string) (*dns.Msg, error) {
	localm.SetQuestion(qname, qtype)

	r, _, err := localc.Exchange(localm, server+":53")
	if err != nil {
		return nil, err
	}
	if r == nil || r.Rcode == dns.RcodeNameError || r.Rcode == dns.RcodeSuccess {
		return r, err
	}

	return nil, errors.New("No name server to answer the question")
}

func getNsRecords(zone string, server string) ([]string, string, error) {
	zone = dns.Fqdn(zone)

	r, err := localQuery(zone, dns.TypeNS, server)
	if err != nil || r == nil {
		return nil, "", err
	}

	var nameservers []string
	var randomNs string

	for _, ans := range r.Answer {
		if t, ok := ans.(*dns.NS); ok {
			nameserver := t.Ns
			nameservers = append(nameservers, nameserver)
		}
	}

	if len(nameservers) == 0 {
		// No "Answer" given by the server, check the Authority section if
		// additional nameservers were provided.
		for _, ans := range r.Ns {
			if t, ok := ans.(*dns.NS); ok {
				nameserver := t.Ns
				nameservers = append(nameservers, nameserver)
			}
		}
	}

	if len(nameservers) == 0 {
		return nil, "", errors.New("No nameservers found for " + zone)
	}

	// Pick a random NS record for the next queries
	randomNs = nameservers[rand.Intn(len(nameservers))]

	sort.Strings(nameservers)

	return nameservers, randomNs, nil
}

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("%s ZONE\n", os.Args[0])
	}
	domain = os.Args[1]

	var err error

	conf, err = dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil || conf == nil {
		log.Fatalf("Cannot initialize the local resolver: %s\n", err)
	}

	localm = &dns.Msg{
		MsgHdr: dns.MsgHdr{
			RecursionDesired: true,
		},
		Question: make([]dns.Question, 1),
	}

	localc = &dns.Client{
		ReadTimeout: DefaultTimeout,
	}

	// Walk the root until you find the authoritative nameservers
	fmt.Println("Retrieving list of root nameservers:")

	rootNameservers, nextNs, err := getNsRecords(".", conf.Servers[0])
	if err != nil {
		log.Fatalf("Query failed: %s", err)
	}

	for _, nameserver := range rootNameservers {
		if nameserver == nextNs {
			// We'll use this one for queries
			fmt.Println(" ➡️ " + nameserver)
		} else {
			fmt.Println(" - " + nameserver)
		}
	}

	// We have list of root nameservers: split domain, query each part for NS records
	domainPieces := dns.SplitDomainName(domain)
	assembledDomain = "."
	var ns []string
	var element string

	// Reverse loop.
	for len(domainPieces) > 0 {
		element = domainPieces[len(domainPieces)-1]
		domainPieces = domainPieces[:len(domainPieces)-1]

		fmt.Print("\n")

		if assembledDomain == "." {
			assembledDomain = element + "."
		} else {
			assembledDomain = element + "." + assembledDomain
		}

		fmt.Println("Finding nameservers for zone '" + assembledDomain + "' using parent nameserver '" + nextNs + "'")
		ns, nextNs, err = getNsRecords(assembledDomain, nextNs)
		if err != nil {
			log.Fatalln("Query failed: ", err)
		}

		// Print the nameservers for this zone, highlight the one we used to query
		for _, nameserver := range ns {
			if nameserver == nextNs && dns.Fqdn(domain) != assembledDomain {
				// We'll use this one for queries
				fmt.Println(" ➡️ " + nameserver)
			} else {
				fmt.Println(" - " + nameserver)
			}
		}
	}
}
