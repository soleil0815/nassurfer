package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// cdp spricht das Chrome DevTools Protocol über die Pipe (--remote-debugging-pipe).
// Nachrichten sind JSON, getrennt durch ein Null-Byte. Kein offener Port.
type cdp struct {
	w       io.Writer
	wmu     sync.Mutex
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan cdpReply
	onEvent func(method, session string, params json.RawMessage)
	closed  chan struct{}
}

type cdpReply struct {
	Result json.RawMessage
	Err    error
}

type cdpMsg struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

var errCDPClosed = errors.New("chromium closed")

func newCDP(r io.Reader, w io.Writer, onEvent func(string, string, json.RawMessage)) *cdp {
	c := &cdp{w: w, pending: map[int64]chan cdpReply{}, onEvent: onEvent, closed: make(chan struct{})}
	go c.readLoop(r)
	return c
}

func (c *cdp) readLoop(r io.Reader) {
	br := bufio.NewReaderSize(r, 1<<20)
	defer func() {
		close(c.closed)
		c.mu.Lock()
		for id, ch := range c.pending {
			ch <- cdpReply{Err: errCDPClosed}
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}()
	for {
		b, err := br.ReadBytes(0)
		if err != nil {
			return
		}
		var m cdpMsg
		if json.Unmarshal(b[:len(b)-1], &m) != nil {
			continue
		}
		if m.ID != 0 {
			c.mu.Lock()
			ch := c.pending[m.ID]
			delete(c.pending, m.ID)
			c.mu.Unlock()
			if ch != nil {
				rep := cdpReply{Result: m.Result}
				if m.Error != nil {
					rep.Err = fmt.Errorf("cdp %d: %s", m.Error.Code, m.Error.Message)
				}
				ch <- rep
			}
			continue
		}
		if m.Method != "" && c.onEvent != nil {
			c.onEvent(m.Method, m.SessionID, m.Params)
		}
	}
}

// call sendet einen Befehl und wartet auf die Antwort.
func (c *cdp) call(session, method string, params any, out any) error {
	ch, err := c.send(session, method, params)
	if err != nil {
		return err
	}
	select {
	case rep := <-ch:
		if rep.Err != nil {
			return rep.Err
		}
		if out != nil && len(rep.Result) > 0 {
			return json.Unmarshal(rep.Result, out)
		}
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("cdp timeout: %s", method)
	case <-c.closed:
		return errCDPClosed
	}
}

// fire sendet einen Befehl, ohne auf die Antwort zu warten (Eingaben, Bestätigungen).
func (c *cdp) fire(session, method string, params any) {
	c.send(session, method, params)
}

func (c *cdp) send(session, method string, params any) (chan cdpReply, error) {
	select {
	case <-c.closed:
		return nil, errCDPClosed
	default:
	}
	p, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if params == nil {
		p = []byte("{}")
	}
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan cdpReply, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	msg, _ := json.Marshal(cdpMsg{ID: id, Method: method, Params: p, SessionID: session})
	c.wmu.Lock()
	_, err = c.w.Write(append(msg, 0))
	c.wmu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	return ch, nil
}
