package main

import (
	"fmt"
	"os"
	"time"

	"github.com/SgtDaJim/op-mtr/mtr"
)

func main() {
	r, err := mtr.RunMTRWithNoRetryPing("0.0.0.0", os.Args[1], 30, 20, 5, time.Second*1)
	j, err2 := r.ToJSON()
	if err != nil {
		fmt.Println(err)
	} else {
		r.PrettyPrint()
	}
	if err2 != nil {
		fmt.Println(err2)
	} else {
		fmt.Println(j)
	}
}
