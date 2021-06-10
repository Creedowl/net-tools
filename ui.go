package main

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"io"
	"os"
	"strconv"
	"sync"
)

var (
	app    *tview.Application
	pages  *tview.Pages
	count  = 0
	m      = sync.Mutex{}
	pinger *Pinger
)

var header = tview.NewFlex().
	AddItem(nil, 0, 1, false).
	AddItem(tview.NewButton("Ping").SetSelectedFunc(func() {
		pages.SwitchToPage("ping")
	}), 0, 2, true).
	AddItem(nil, 0, 1, false).
	AddItem(tview.NewButton("Traceroute").SetSelectedFunc(func() {
		pages.SwitchToPage("trace")
	}), 0, 2, false).
	AddItem(nil, 0, 1, false).
	AddItem(tview.NewButton("Scan").SetSelectedFunc(func() {
		pages.SwitchToPage("scan")
	}), 0, 2, false).
	AddItem(nil, 0, 1, false)

func Show() {
	app = tview.NewApplication().EnableMouse(true)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			switch app.GetFocus().(type) {
			case *tview.InputField:
			default:
				pages.SwitchToPage("quit")
			}
		case ':':
			app.SetFocus(pages)
		}
		return event
	})

	pages = tview.NewPages()
	pages.AddPage(newPingPage()).AddPage(newQuitPage()).AddPage(newScanPage()).AddPage(newTracePage())

	err := app.SetRoot(pages, true).Run()
	if err != nil {
		fmt.Printf("failed to run app: %s\n", err)
		return
	}
}

func checkNum(_ string, lastChar rune) bool {
	if lastChar < '0' || lastChar > '9' {
		return false
	}
	return true
}

func fprintln(w io.Writer, a ...interface{}) {
	_, err := fmt.Fprintln(w, a...)
	if err != nil {
		fmt.Println("failed to write text to text view")
		os.Exit(1)
	}
	app.Draw()
}

func output(textView *tview.TextView, ch chan string) {
	go func() {
		app.QueueUpdate(func() {
			textView.Clear()
		})
		for s := range ch {
			fprintln(textView, s)
			app.Draw()
		}
	}()
}

func ping(host string, repeat, timeout int, textView *tview.TextView) {
	m.Lock()
	if count > 0 {
		m.Unlock()
		return
	}
	count++
	m.Unlock()
	ch := make(chan string, 10)
	output(textView, ch)
	ch <- host
	_pinger, err := NewPinger(host, repeat, timeout, ch)
	if err != nil {
		fprintln(textView, err)
	} else {
		m.Lock()
		pinger = _pinger
		m.Unlock()
		pinger.Ping()
		close(ch)
	}
	m.Lock()
	pinger = nil
	count--
	m.Unlock()
}

func scan(cidr string, textView *tview.TextView, resultView *tview.TextView) {
	ch := make(chan string)
	defer close(ch)
	output(textView, ch)
	scanner, err := NewScanner(cidr, ch)
	if err != nil {
		fprintln(textView, err)
		return
	}
	result := scanner.Scan()
	resultView.Clear()
	for _, ip := range result {
		fprintln(resultView, ip)
	}
	app.Draw()
}

func trace(host string, textView *tview.TextView) {
	ch := make(chan string)
	defer close(ch)
	output(textView, ch)
	tracer, err := NewTracer(host, ch)
	if err != nil {
		fprintln(textView, err)
		return
	}
	tracer.Trace()
}

func pause() {
	m.Lock()
	defer m.Unlock()
	if pinger == nil {
		return
	}
	pinger.Pause()
}

func resume() {
	m.Lock()
	defer m.Unlock()
	if pinger == nil {
		return
	}
	pinger.Resume()
}

func cancel() {
	m.Lock()
	defer m.Unlock()
	if pinger == nil {
		return
	}
	pinger.Cancel()
}

func newPingPage() (string, tview.Primitive, bool, bool) {
	host := "localhost"
	repeat := 5
	timeout := 5

	flex := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(header, 3, 0, true)
	flex.SetBorder(true).SetTitle("Net Tools")

	container := tview.NewFlex()
	container.SetBorder(true).SetTitle("Ping")

	textview := tview.NewTextView()
	textview.SetBorder(true).SetTitle("result")
	textview.ScrollToEnd()

	form := tview.NewForm()
	form.
		AddInputField("host", host, 0, nil, func(text string) {
			host = text
		}).
		AddInputField("repeat", strconv.Itoa(repeat), 0, checkNum, func(text string) {
			_repeat, _ := strconv.ParseInt(text, 10, 32)
			repeat = int(_repeat)
		}).
		AddInputField("timeout", strconv.Itoa(timeout), 0, checkNum, func(text string) {
			_timeout, _ := strconv.ParseInt(text, 10, 32)
			timeout = int(_timeout)
		}).
		AddButton("Ping", func() {
			go ping(host, repeat, timeout, textview)
		}).
		AddButton("Pause", func() {
			go pause()
		}).
		AddButton("Resume", func() {
			go resume()
		}).
		AddButton("Cancel", func() {
			go cancel()
		})

	container.AddItem(form, 0, 1, false).AddItem(textview, 0, 1, false)

	flex.AddItem(container, 0, 1, false)
	return "ping", flex, true, true
}

func newQuitPage() (string, tview.Primitive, bool, bool) {
	modal := tview.NewModal().
		SetText("Do you want to quit the application?").
		AddButtons([]string{"Quit", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Quit" {
				app.Stop()
			}
			pages.SwitchToPage("ping")
		})
	return "quit", modal, true, false
}

func newScanPage() (string, tview.Primitive, bool, bool) {
	cidr := "192.168.1.1/24"

	flex := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(header, 3, 0, true)
	flex.SetBorder(true).SetTitle("Net Tools")

	container := tview.NewFlex()
	container.SetBorder(true).SetTitle("Scan")

	textview := tview.NewTextView()
	textview.SetBorder(true).SetTitle("log")
	textview.ScrollToEnd()

	resultView := tview.NewTextView()
	resultView.SetBorder(true).SetTitle("Available subnets")
	resultView.ScrollToEnd()

	form := tview.NewForm()
	form.
		AddInputField("CIDR", cidr, 20, nil, func(text string) {
			cidr = text
		}).
		AddButton("Scan", func() {
			go scan(cidr, textview, resultView)
		})

	container.
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(form, 0, 1, true).
				AddItem(textview, 0, 2, false), 0, 1, false).
		AddItem(resultView, 0, 1, false)

	flex.AddItem(container, 0, 1, false)
	return "scan", flex, true, false
}

func newTracePage() (string, tview.Primitive, bool, bool) {
	host := "baidu.com"

	flex := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(header, 3, 0, true)
	flex.SetBorder(true).SetTitle("Net Tools")

	container := tview.NewFlex()
	container.SetBorder(true).SetTitle("Traceroute")

	textview := tview.NewTextView()
	textview.SetBorder(true).SetTitle("result")
	textview.ScrollToEnd()

	form := tview.NewForm()
	form.
		AddInputField("host", host, 0, nil, func(text string) {
			host = text
		}).
		AddButton("Trace", func() {
			go trace(host, textview)
		})

	container.AddItem(form, 0, 1, false).AddItem(textview, 0, 1, false)

	flex.AddItem(container, 0, 1, false)

	return "trace", flex, true, false
}
