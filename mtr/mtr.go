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

	"github.com/pixelbender/go-traceroute/traceroute"
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

type OPMTR struct {
	Tracer      *traceroute.Tracer
	MaxUnknowns int
	PingCount   int
}

func NewOPMTR(src string, maxHops, count, maxUnknowns int, timeout time.Duration) (*OPMTR, error) {
	srcIP := net.ParseIP(src)
	if srcIP == nil {
		return nil, errors.New("Unknown source IP")
	}
	op := &OPMTR{
		Tracer: &traceroute.Tracer{
			Config: traceroute.Config{
				Delay:    10 * time.Millisecond,
				Timeout:  timeout,
				MaxHops:  maxHops,
				Count:    1,
				Networks: []string{"ip4:icmp"},
				Addr:     &net.IPAddr{IP: srcIP},
			},
		},
		MaxUnknowns: maxUnknowns,
		PingCount:   count,
	}
	return op, nil
}

func (op *OPMTR) Close() {
	op.Tracer.Close()
}

func (op *OPMTR) RunMTRWithNoRetryPing(dst string) (MTRReport, error) {
	dstIP := net.ParseIP(dst)
	if dstIP == nil {
		return MTRReport{}, errors.New("Unknown dest IP")
	}
	report := MTRReport{
		Src:   op.Tracer.Addr.String(),
		Dst:   dst,
		Count: op.PingCount,
	}
	routes := map[int]*traceroute.Reply{}
	if err := op.Tracer.Trace(context.Background(), dstIP, func(reply *traceroute.Reply) {
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
	for i := 1; i <= op.Tracer.MaxHops; i++ {
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
			if r.IP.String() == dstIP.String() {
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
		if unknownCount >= op.MaxUnknowns {
			break
		}
		if h.Host == dst {
			break
		}
	}

	// ping
	hupsLen := len(hups)
	for i := 1; i <= hupsLen; i++ {
		hup := hups[i]
		for j := 1; j <= op.PingCount-1; j++ {
			var rp *traceroute.Reply
			var err error
			hup.Snt++
			if hup.Host != "???" {
				rp, err = ping(op.Tracer, hup.Host, op.Tracer.MaxHops, op.Tracer.Timeout)
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
			}
		}
		// log.Println(hup)
		if hup.Host != "???" {
			hup.Loss = float64(hup.LossPoint) / float64(hup.Snt)
		}

	}

	for i := 1; i <= hupsLen; i++ {
		v := hups[i]
		report.Hups = append(report.Hups, *v)
	}
	return report, nil
}

func (op *OPMTR) RunMTR(dst string) (MTRReport, error) {
	dstIP := net.ParseIP(dst)
	if dstIP == nil {
		return MTRReport{}, errors.New("Unknown dest IP")
	}
	report := MTRReport{
		Src:   op.Tracer.Addr.String(),
		Dst:   dst,
		Count: op.PingCount,
	}
	routes := map[int]*traceroute.Reply{}
	if err := op.Tracer.Trace(context.Background(), dstIP, func(reply *traceroute.Reply) {
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
	for i := 1; i <= op.Tracer.MaxHops; i++ {
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
			if r.IP.String() == dstIP.String() {
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
		if unknownCount >= op.MaxUnknowns {
			break
		}
		if h.Host == dst {
			break
		}
	}

	// ping cocurrently
	hupsLen := len(hups)
	for i := 1; i <= hupsLen; i++ {
		hup := hups[i]
		to := op.Tracer.Timeout
		var retryTime int
		var workTimeout time.Duration
		var comeback bool
		for j := 1; j <= op.PingCount-1; j++ {
			var rp *traceroute.Reply
			var err error
			hup.Snt++
			if hup.Host != "???" {
				if comeback {
					rp, err = ping(op.Tracer, hup.Host, hup.Count, workTimeout)
				} else {
					rp, err = ping(op.Tracer, hup.Host, op.Tracer.MaxHops, op.Tracer.Timeout)
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
				if rp, err = ping(op.Tracer, dstIP.String(), hup.Count, to); err == nil && rp != nil {
					hupsCopy := hups
					toComeback := true
					for _, v := range hupsCopy {
						if v.Host == rp.IP.String() {
							toComeback = false
							break
						}
					}
					if toComeback {
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
						if to < time.Second*5 {
							to += time.Second
						}
						hup.LossPoint++
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

	}

	for i := 1; i <= hupsLen; i++ {
		v := hups[i]
		// log.Printf("Count: %v, Host: %v, Snt: %v, Loss: %.2f, Last: %v, Avg: %v, Best: %v, Wrst: %v", v.Count, v.Host, v.Snt, v.Loss, v.Last, v.Avg, v.Best, v.Wrst)
		report.Hups = append(report.Hups, *v)
	}
	return report, nil
}

func (op *OPMTR) RunMTRWithCocurrentPing(dst string) (MTRReport, error) {
	dstIP := net.ParseIP(dst)
	if dstIP == nil {
		return MTRReport{}, errors.New("Unknown dest IP")
	}
	report := MTRReport{
		Src:   op.Tracer.Addr.String(),
		Dst:   dst,
		Count: op.PingCount,
	}
	routes := map[int]*traceroute.Reply{}
	if err := op.Tracer.Trace(context.Background(), dstIP, func(reply *traceroute.Reply) {
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
	for i := 1; i <= op.Tracer.MaxHops; i++ {
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
			if r.IP.String() == dstIP.String() {
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
		if unknownCount >= op.MaxUnknowns {
			break
		}
		if h.Host == dst {
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
			to := op.Tracer.Timeout
			var retryTime int
			var workTimeout time.Duration
			var comeback bool
			for j := 1; j <= op.PingCount-1; j++ {
				var rp *traceroute.Reply
				var err error
				hup.Snt++
				if hup.Host != "???" {
					if comeback {
						rp, err = ping(op.Tracer, hup.Host, hup.Count, workTimeout)
					} else {
						rp, err = ping(op.Tracer, hup.Host, op.Tracer.MaxHops, op.Tracer.Timeout)
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
					if rp, err = ping(op.Tracer, dstIP.String(), hup.Count, to); err == nil && rp != nil {
						hupsCopy := hups
						toComeback := true
						for _, v := range hupsCopy {
							if v.Host == rp.IP.String() {
								toComeback = false
								break
							}
						}
						if toComeback {
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
							if to < time.Second*5 {
								to += time.Second
							}
							hup.LossPoint++
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

func ping(t *traceroute.Tracer, ip string, ttl int, timeout time.Duration) (r *traceroute.Reply, err error) {
	sess, err := t.NewSession(net.ParseIP(ip))
	if err != nil {
		return
	}
	defer sess.Close()
	err = sess.Ping(ttl)
	if err != nil {
		return
	}
	select {
	case r = <-sess.Receive():
		return
	case <-time.After(timeout):
		return
	}

}

// ToJSON convert struct to JSON String
func (r MTRReport) ToJSON() (string, error) {
	if b, err := json.Marshal(r); err != nil {
		return "", err
	} else {
		return string(b), nil
	}
}

// ToRawJSON convert struct to JSON Object (Bytes)
func (r MTRReport) ToRawJSON() ([]byte, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// PrettyPrint print the MTR report in format
func (r MTRReport) PrettyPrint() {
	fmt.Printf("Time: %s\tSrc: %s\tDst: %s\tCount: %d\n", time.Unix(r.Time, 0).String(), r.Src, r.Dst, r.Count)
	fmt.Printf("%4s    %-20s %5s%%  %4s  %6s  %6s  %6s  %6s\n", "HOP:|", "Address", "Loss", "Snt", "Last", "Avg", "Best", "Wrst")
	for _, h := range r.Hups {
		if h.Host != "???" {
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
		} else {
			fmt.Printf("%3d:|-- %-20s\n",
				h.Count,
				h.Host,
			)
		}
	}
}
