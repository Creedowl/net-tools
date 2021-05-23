package main

import (
	"math/rand"
	"time"
)

//func main() {
//	ch := make(chan string, 5)
//	wg := sync.WaitGroup{}
//	wg.Add(1)
//	go func() {
//		for s := range ch {
//			fmt.Println(s)
//		}
//		wg.Done()
//	}()
//	pinger, err := NewPinger("localhost", 5, 5, ch)
//	if err != nil {
//		fmt.Println(err)
//		wg.Done()
//		return
//	}
//	pinger.Ping()
//	wg.Wait()
//}

func main() {
	rand.Seed(time.Now().Unix())
	Show()
	//ch := make(chan string)
	//scanner, err := NewScanner("192.168.1.1/24", ch)
	//if err != nil {
	//	fmt.Println(err)
	//	return
	//}
	//go func() {
	//	for s := range ch {
	//		fmt.Println(s)
	//	}
	//}()
	//res := scanner.Scan()
	//fmt.Println(res)
}
