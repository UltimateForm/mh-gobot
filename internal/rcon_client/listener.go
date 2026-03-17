package rcon_client

import (
	"context"
	"errors"
	"log"
	"net"
	"strings"
	"time"

	"github.com/UltimateForm/ryard/internal/parse"
	"github.com/UltimateForm/tcprcon/pkg/packet"
)

type ListenType string

const (
	ListenLogin      ListenType = "login"
	ListenKillfeed   ListenType = "killfeed"
	ListenScorefeed  ListenType = "scorefeed"
	ListenChat       ListenType = "chat"
	ListenMatchstate ListenType = "matchstate"
	ListenAll        ListenType = "allon"
)

const listenerChannelBuffer = 32
const keepaliveIntervalSecs = 100
const reconnectDelaySecs = 5

type ListenerClient struct {
	client                *ControlledClient
	uri                   string
	password              string
	listenTypes           []ListenType
	KillfeedEvents        <-chan *parse.KillfeedEvent
	LoginEvents           <-chan *parse.LoginEvent
	ChatEvents            <-chan *parse.ChatEvent
	ScorefeedPlayerEvents <-chan *parse.ScorefeedPlayerEvent
	ScorefeedTeamEvents   <-chan *parse.ScorefeedTeamEvent
	MatchstateEvents      <-chan string
	killfeedCh            chan *parse.KillfeedEvent
	loginCh               chan *parse.LoginEvent
	chatCh                chan *parse.ChatEvent
	scorefeedPlayerCh     chan *parse.ScorefeedPlayerEvent
	scorefeedTeamCh       chan *parse.ScorefeedTeamEvent
	matchstateCh          chan string
	logger                *log.Logger
}

func NewListener(uri, password string, listenTypes []ListenType) (*ListenerClient, error) {
	base, err := New(uri)
	if err != nil {
		return nil, err
	}
	success, err := base.Authenticate(password)
	if err != nil {
		return nil, err
	}
	if !success {
		return nil, errors.New("authentication failed")
	}

	l := &ListenerClient{
		client:            base,
		uri:               uri,
		password:          password,
		listenTypes:       listenTypes,
		killfeedCh:        make(chan *parse.KillfeedEvent, listenerChannelBuffer),
		loginCh:           make(chan *parse.LoginEvent, listenerChannelBuffer),
		chatCh:            make(chan *parse.ChatEvent, listenerChannelBuffer),
		scorefeedPlayerCh: make(chan *parse.ScorefeedPlayerEvent, listenerChannelBuffer),
		scorefeedTeamCh:   make(chan *parse.ScorefeedTeamEvent, listenerChannelBuffer),
		matchstateCh:      make(chan string, listenerChannelBuffer),
		logger: log.New(
			log.Default().Writer(),
			"[ListenerClient] ",
			log.Default().Flags(),
		),
	}
	l.KillfeedEvents = l.killfeedCh
	l.LoginEvents = l.loginCh
	l.ChatEvents = l.chatCh
	l.ScorefeedPlayerEvents = l.scorefeedPlayerCh
	l.ScorefeedTeamEvents = l.scorefeedTeamCh
	l.MatchstateEvents = l.matchstateCh

	for _, t := range listenTypes {
		resp, err := base.Execute("listen " + string(t))
		if err != nil {
			return nil, errors.Join(errors.New("failed to register listener "+string(t)), err)
		}
		l.logger.Printf("listen %s: %s", t, resp)
	}

	return l, nil
}

func (l *ListenerClient) reconnect() error {
	l.client.Close()
	base, err := New(l.uri)
	if err != nil {
		return err
	}
	success, err := base.Authenticate(l.password)
	if err != nil {
		return err
	}
	if !success {
		return errors.New("authentication failed")
	}
	for _, t := range l.listenTypes {
		resp, err := base.Execute("listen " + string(t))
		if err != nil {
			return errors.Join(errors.New("failed to re-register listener "+string(t)), err)
		}
		l.logger.Printf("re-registered listen %s: %s", t, resp)
	}
	l.client = base
	return nil
}

func (l *ListenerClient) route(body string) {
	switch {
	case strings.HasPrefix(body, "Killfeed:"):
		event, err := parse.ParseKillfeedEvent(body)
		if err != nil || event == nil {
			l.logger.Printf("failed to parse killfeed event: %v", err)
			return
		}
		select {
		case l.killfeedCh <- event:
		default:
			l.logger.Println("killfeedCh full, dropping event")
		}
	case strings.HasPrefix(body, "Login:"), strings.HasPrefix(body, "Logout:"):
		event, err := parse.ParseLoginEvent(body)
		if err != nil || event == nil {
			l.logger.Printf("failed to parse login event: %v", err)
			return
		}
		select {
		case l.loginCh <- event:
		default:
			l.logger.Println("loginCh full, dropping event")
		}
	case strings.HasPrefix(body, "Chat:"):
		event, err := parse.ParseChatEvent(body)
		if err != nil || event == nil {
			l.logger.Printf("failed to parse chat event: %v", err)
			return
		}
		select {
		case l.chatCh <- event:
		default:
			l.logger.Println("chatCh full, dropping event")
		}
	case strings.HasPrefix(body, "Scorefeed:"):
		playerEvent, teamEvent, err := parse.ParseScorefeedEvent(body)
		if err != nil {
			l.logger.Printf("failed to parse scorefeed event: %v", err)
			return
		}
		if playerEvent != nil {
			select {
			case l.scorefeedPlayerCh <- playerEvent:
			default:
				l.logger.Println("scorefeedPlayerCh full, dropping event")
			}
		} else if teamEvent != nil {
			select {
			case l.scorefeedTeamCh <- teamEvent:
			default:
				l.logger.Println("scorefeedTeamCh full, dropping event")
			}
		}
	case strings.HasPrefix(body, "MatchState:"):
		state, err := parse.ParseMatchstate(body)
		if err != nil {
			l.logger.Printf("failed to parse matchstate: %v", err)
			return
		}
		select {
		case l.matchstateCh <- state:
		default:
			l.logger.Println("matchstateCh full, dropping event")
		}
	default:
		l.logger.Printf("unrouted event: %s", body)
	}
}

func (l *ListenerClient) stream(ctx context.Context) {
	for {
		connCtx, cancelConn := context.WithCancel(ctx)
		go l.keepalive(connCtx)

		packets := packet.CreateResponseChannel(l.client, connCtx)
		for pkt := range packets {
			if pkt.Error != nil {
				if netErr, ok := pkt.Error.(net.Error); ok && netErr.Timeout() {
					continue
				}
				l.logger.Printf("stream error: %v", pkt.Error)
				break
			}
			body := pkt.BodyStr()
			if strings.HasPrefix(body, "Keeping client alive") {
				continue
			}
			l.route(body)
		}

		cancelConn()

		if ctx.Err() != nil {
			return
		}

		l.logger.Println("connection lost, reconnecting...")
		for {
			time.Sleep(reconnectDelaySecs * time.Second)
			if err := l.reconnect(); err != nil {
				l.logger.Printf("reconnect failed: %v, retrying...", err)
				continue
			}
			l.logger.Println("reconnected successfully")
			break
		}
	}
}

func (l *ListenerClient) keepalive(ctx context.Context) {
	ticker := time.NewTicker(keepaliveIntervalSecs * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.client.mu.Lock()
			id := l.client.Id()
			pkt := packet.New(id, packet.SERVERDATA_EXECCOMMAND, []byte("alive"))
			_, err := l.client.Write(pkt.Serialize())
			l.client.mu.Unlock()
			if err != nil {
				l.logger.Printf("keepalive write error: %v", err)
			}
		}
	}
}

func (l *ListenerClient) Run(ctx context.Context) {
	go l.stream(ctx)
}
