package main

import (
    "fmt"
    "github.com/miekg/dns"
    "log"
    "net"
    "time"
)

// StartDNSServer launches a UDP DNS server at addr (e.g. ":53") using the provided BlocklistManager.
func StartDNSServer(addr string, bm *BlocklistManager) error {
    dns.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
        msg := dns.Msg{}
        msg.SetReply(r)
        msg.Authoritative = true

        for _, q := range r.Question {
            qname := q.Name
            // log client address and query name
            clientAddr := "unknown"
            if ra := w.RemoteAddr(); ra != nil {
                clientAddr = ra.String()
            }
            fmt.Printf("Received query for %s from %s\n", qname, clientAddr)
            // normalize
            name := qname
            if len(name) > 0 && name[len(name)-1] == '.' {
                name = name[:len(name)-1]
            }

            if bm.IsBlocked(name) {
                // Depending on blocking mode, reply differently
                switch AppConfig.BlockingMode {
                case "redirect":
                    // return A record pointing to the block page IP so browsers hit the block page server
                    target := AppConfig.BlockPageIP
                    if target == "" {
                        target = "127.0.0.1"
                    }
                    if q.Qtype == dns.TypeA || q.Qtype == dns.TypeANY {
                        a := new(dns.A)
                        a.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}
                        a.A = net.ParseIP(target)
                        msg.Answer = append(msg.Answer, a)
                    }
                case "nx":
                    // NXDOMAIN
                    msg.Rcode = dns.RcodeNameError
                default:
                    // null route (0.0.0.0)
                    if q.Qtype == dns.TypeA || q.Qtype == dns.TypeANY {
                        a := new(dns.A)
                        a.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0}
                        a.A = net.ParseIP("0.0.0.0")
                        msg.Answer = append(msg.Answer, a)
                    }
                }
                // record analytics and write reply and stop processing
                bm.RecordQueryWithClient(name, clientAddr, true)
                log.Printf("blocked %s for client %s (mode=%s)", name, clientAddr, AppConfig.BlockingMode)
                _ = w.WriteMsg(&msg)
                return
            }

            // forward the query upstream (configured or Cloudflare by default)
            upstream := AppConfig.Upstream
            if upstream == "" {
                upstream = "1.1.1.1:53"
            }
            c := new(dns.Client)
            c.ReadTimeout = 5 * time.Second
            resp, _, err := c.Exchange(r, upstream)
            if err == nil && resp != nil {
                msg.Answer = append(msg.Answer, resp.Answer...)
            }
            // record allowed query
            bm.RecordQueryWithClient(name, clientAddr, false)
            log.Printf("allowed %s for client %s", name, clientAddr)
        }

        _ = w.WriteMsg(&msg)
    })

    server := &dns.Server{Addr: addr, Net: "udp"}
    return server.ListenAndServe()
}
