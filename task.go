package market

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	"time"
)

const maxRetryNum = 10
const runIng = 1

type (
	Handler interface {
		formatSubscribeHandle(*Subscriber) []byte
		pingPongHandle(*Worker)
		resubscribeHandle(*Worker)
		formatMsgHandle(int, []byte, *Worker) (*Marketer, error)
		subscribed(msg []byte, worker *Worker)
	}

	Task interface {
		RunTask()
	}

	Worker struct {
		ctx              context.Context
		wsUrl            string
		Organize         Organize
		Status           int
		LastRunTimestamp time.Duration
		WsConn           *websocket.Conn
		Subscribing      map[string][]byte
		Subscribes       map[string][]byte
		List             Lister
		handler          Handler
	}
)

func (w *Worker) RunTask() {
	var err error

	w.WsConn, err = dial(w.wsUrl)
	if err != nil {
		panic(err)
	}
	defer w.WsConn.Close()

	w.listenHandle()
}

func (w *Worker) Subscribe(msg []byte) error {
	if w.WsConn != nil {
		err := w.WsConn.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *Worker) closeRedialSub() error {
	var err error
	w.WsConn.Close()
	w.WsConn, err = dial(w.wsUrl)
	for _, msg := range w.Subscribes {
		w.Subscribe(msg)
	}
	return err
}

func (w *Worker) subscribeHandle(s *Subscriber) {
	w.Subscribing[s.Symbol] = w.handler.formatSubscribeHandle(s)
	w.Subscribe(w.Subscribing[s.Symbol])
}

func (w *Worker) subscribed(symbol string) {
	if sub, ok := w.Subscribing[symbol]; ok {
		w.Subscribes[symbol] = sub
		delete(w.Subscribing, symbol)
	}
}

func (w *Worker) listenHandle() {
	go w.handler.pingPongHandle(w)
	go w.handler.resubscribeHandle(w)
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			msgType, msg, err := w.WsConn.ReadMessage()
			if err != nil {
				err = w.closeRedialSub()
				if err != nil {
					return
				}

				msgType, msg, err = w.WsConn.ReadMessage()
				if err != nil {
					return
				}
			}

			data, err := w.handler.formatMsgHandle(msgType, msg, w)
			if data != nil {
				w.List.Add(data.Symbol, data)
			}
		}
	}
}

func dial(u string) (*websocket.Conn, error) {
	var retryNum int

RETRY:
	if retryNum > maxRetryNum {
		return nil, errors.New("connect fail")
	}

	uProxy, _ := url.Parse("http://127.0.0.1:12333")

	websocket.DefaultDialer = &websocket.Dialer{
		Proxy:            http.ProxyURL(uProxy),
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		retryNum++
		time.Sleep(time.Second)
		goto RETRY
	}

	return conn, nil
}