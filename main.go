package main

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
	Show()
}
