package main

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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
	AddItem(tview.NewButton("Ping"), 0, 2, true).
	AddItem(nil, 0, 1, false).
	AddItem(tview.NewButton("Traceroute"), 0, 2, false).
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
	pages.AddPage(newPingPage()).AddPage(newQuitPage())

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

func ping(host string, repeat, timeout int, textView *tview.TextView) {
	m.Lock()
	if count > 0 {
		m.Unlock()
		return
	}
	count++
	m.Unlock()
	ch := make(chan string, 10)
	go func() {
		app.QueueUpdate(func() {
			textView.Clear()
		})
		for s := range ch {
			_, err := fmt.Fprintln(textView, s)
			if err != nil {
				fmt.Println("failed to write text to text view")
				os.Exit(1)
			}
			app.Draw()
		}
		m.Lock()
		pinger = nil
		m.Unlock()
	}()
	_pinger, err := NewPinger(host, repeat, timeout, ch)
	if err != nil {
		_, err := fmt.Fprintln(textView, err)
		if err != nil {
			fmt.Println("failed to write text to text view")
			os.Exit(1)
		}
		return
	}
	m.Lock()
	pinger = _pinger
	m.Unlock()
	pinger.Ping()
	close(ch)
	m.Lock()
	count--
	m.Unlock()
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
	flex.SetBorder(true).SetTitle("Ping & Traceroute")

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
