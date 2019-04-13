package mtr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/SgtDaJim/go-traceroute/traceroute"
)

type MTRReport struct {
	Time  int64    `json:"ts"`
	Src   string   `json:"src"`
	Dst   string   `json:"dst"`
	Count int      `json:"count"`
	Hups  []MTRHup `json:"hups"`
}

type MTRHup struct {
	Count     int     `json:"count"`
	Host      string  `json:"host"`
	Loss      float64 `json:"Loss"`
	LossPoint int     `json:"-"`
	Snt       float64 `json:"Snt"`
	Last      float64 `json:"Last"`
	Avg       float64 `json:"Avg"`
	Best      float64 `json:"Best"`
	Wrst      float64 `json:"Wrst"`
}

func RunMTR(src, dst string, maxHops, count, maxUnknowns int, timeout time.Duration) (MTRReport, error) {
	srcIP := net.ParseIP(src)
	if srcIP == nil {
		return MTRReport{}, errors.New("Unknown source IP")
	}
	dstIP := net.ParseIP(dst)
	if dstIP == nil {
		return MTRReport{}, errors.New("Unknown dest IP")
	}
	report := MTRReport{
		Src:   src,
		Dst:   dst,
		Count: count,
	}
	t := &traceroute.Tracer{
		Config: traceroute.Config{
			Delay:    10 * time.Millisecond,
			Timeout:  timeout,
			MaxHops:  maxHops,
			Count:    1,
			Networks: []string{"ip4:icmp"},
			Addr:     &net.IPAddr{IP: srcIP},
		},
	}
	defer t.Close()
	routes := map[int]*traceroute.Reply{}
	if err := t.Trace(context.Background(), dstIP, func(reply *traceroute.Reply) {
		if ex, ok := routes[reply.Hops]; ok {
			log.Printf("Conflict. Hop: %v, newIP: %v, exIP: %v", reply.Hops, reply.IP, ex.IP)
		} else {
			routes[reply.Hops] = reply
		}
	}); err != nil {
		return report, err
	}

	report.Time = time.Now().Unix()
	// trace first
	hups := map[int]*MTRHup{}
	var unknownCount int
	for i := 1; i <= maxHops; i++ {
		if r, ok := routes[i]; ok {
			rtt := r.RTT.Seconds() * 1000
			hups[i] = &MTRHup{
				Count:     i,
				Host:      r.IP.String(),
				Snt:       1,
				LossPoint: 0,
				Last:      rtt,
				Avg:       rtt,
				Best:      rtt,
				Wrst:      rtt,
			}
			unknownCount = 0
			if r.IP.String() == dst {
				break
			}
		} else {
			hups[i] = &MTRHup{
				Count:     i,
				Host:      "???",
				Snt:       1,
				LossPoint: 1,
				Last:      0,
				Avg:       0,
				Best:      0,
				Wrst:      0,
			}
			unknownCount++
		}
		h := hups[i]
		h.Loss = float64(h.LossPoint) / float64(h.Snt)
		hups[i] = h
		if unknownCount >= maxUnknowns {
			break
		}
	}

	// ping cocurrently
	hupsLen := len(hups)
	var wg sync.WaitGroup
	for i := 1; i <= hupsLen; i++ {
		wg.Add(1)
		hup := hups[i]
		go func() {
			defer wg.Done()
			to := t.Timeout
			var retryTime int
			var workTimeout time.Duration
			var comeback bool
			for j := 1; j <= count-1; j++ {
				var rp *traceroute.Reply
				hup.Snt++
				if hup.Host != "???" {
					var err error
					if comeback {
						err = t.StaticTrace(context.Background(), net.ParseIP(hup.Host), hup.Count, workTimeout, func(reply *traceroute.Reply) {
							rp = reply
						})
					} else {
						err = t.Ping(context.Background(), net.ParseIP(hup.Host), func(reply *traceroute.Reply) {
							rp = reply
						})
					}
					if err == nil && rp != nil {
						rtt := rp.RTT.Seconds() * 1000
						hup.Last = rtt
						hup.Avg = (hup.Avg*(hup.Snt-1) + rtt) / hup.Snt
						if hup.Best > rtt {
							hup.Best = rtt
						}
						if hup.Wrst < rtt {
							hup.Wrst = rtt
						}
					} else {
						if err != nil {
							log.Println(err)
						}
						hup.LossPoint++
					}
				} else {
					if retryTime >= 4 {
						continue
					}
					if err := t.StaticTrace(context.Background(), dstIP, hup.Count, to, func(reply *traceroute.Reply) {
						rp = reply
					}); err == nil && rp != nil {
						comeback = true
						workTimeout = to
						hup.Host = rp.IP.String()
						rtt := rp.RTT.Seconds() * 1000
						hup.Last = rtt
						hup.Avg = (hup.Avg*(hup.Snt-1) + rtt) / hup.Snt
						if hup.Best > rtt {
							hup.Best = rtt
						}
						if hup.Wrst < rtt {
							hup.Wrst = rtt
						}
					} else {
						if err != nil {
							log.Println(err)
						}
						if to < time.Second*5 {
							to += time.Second
						}
						hup.LossPoint++
					}
					retryTime++
				}
			}
			// log.Println(hup)
			if hup.Host != "???" {
				hup.Loss = float64(hup.LossPoint) / float64(hup.Snt)
			}
		}()
	}

	wg.Wait()

	for i := 1; i <= hupsLen; i++ {
		v := hups[i]
		// log.Printf("Count: %v, Host: %v, Snt: %v, Loss: %.2f, Last: %v, Avg: %v, Best: %v, Wrst: %v", v.Count, v.Host, v.Snt, v.Loss, v.Last, v.Avg, v.Best, v.Wrst)
		report.Hups = append(report.Hups, *v)
	}
	return report, nil
}

// ToJSON convert struct to JSON
func (r MTRReport) ToJSON() (string, error) {
	if b, err := json.Marshal(r); err != nil {
		return "", err
	} else {
		return string(b), nil
	}
}

// PrettyPrint print the MTR report in format
func (r MTRReport) PrettyPrint() {
	fmt.Printf("Time: %s\tSrc: %s\tDst: %s\tCount: %d\n", time.Unix(r.Time, 0).String(), r.Src, r.Dst, r.Count)
	fmt.Printf("%4s    %-20s %5s%%  %4s  %6s  %6s  %6s  %6s\n", "HOP:|", "Address", "Loss", "Snt", "Last", "Avg", "Best", "Wrst")
	for _, h := range r.Hups {
		fmt.Printf("%3d:|-- %-20s %5.1f%%  %4v  %6.1f  %6.1f  %6.1f  %6.1f\n",
			h.Count,
			h.Host,
			h.Loss*100.0,
			h.Snt,
			h.Last,
			h.Avg,
			h.Best,
			h.Wrst,
		)
	}
}
