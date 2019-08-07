package main

import (
	"fmt"
	"os"
	"time"

	"github.com/SgtDaJim/op-mtr/mtr"
)

func main() {
	opmtr, err1 := mtr.NewOPMTR("0.0.0.0", 30, 20, 5, time.Second*1)
	if err1 != nil {
		fmt.Println(err1)
		return
	}
	r, err := opmtr.RunMTRWithCocurrentPing(os.Args[1])
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
