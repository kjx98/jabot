package jabot

import (
	"encoding/xml"
	"errors"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kjx98/go-xmpp"
	//"github.com/kjx98/golib/to"
	"github.com/op/go-logging"
)

type Jabot struct {
	cfg        Config
	nickName   string
	resource   string
	client     *xmpp.Client
	auto       bool
	bConnected bool
	lastAct    time.Time
	contacts   map[string]Contact
}

type Contact struct {
	Jid          string
	Name         string
	NickName     string
	Group        []string
	Online       bool
	Subscription string
}

var (
	errLoginFail    = errors.New("Login failed")
	errLoginTimeout = errors.New("Login time out")
	errNoConn       = errors.New("No connection to server")
	errHandleExist  = errors.New("命令处理器已经存在")
)
var log = logging.MustGetLogger("jabot")

// HandleFunc type
//	used for RegisterHandle
type HandlerFunc func(args []string) string

var handlers = map[string]HandlerFunc{}

func NewJabot(cfg *Config) (*Jabot, error) {
	rand.Seed(time.Now().Unix())
	randID := strconv.Itoa(rand.Int())

	wx := Jabot{
		cfg:      *cfg,
		resource: "ebot" + randID[2:17],
		contacts: make(map[string]Contact),
		auto:     true,
	}
	return &wx, nil
}

//go: noinline
func defTimeFunc(args []string) string {
	tt := time.Now()
	if len(args) > 0 && strings.ToUpper(strings.Trim(args[0], " \t")) == "UTC" {
		tt = tt.UTC()
	}
	return tt.Format("01-02 15:04:05")
}

func (w *Jabot) RegisterTimeCmd() {
	w.RegisterHandle("time", defTimeFunc)
	w.RegisterHandle("时间", defTimeFunc)
}

// SetLogLevel
//	logging.Level   from github.com/op/go-logging
func (w *Jabot) SetLogLevel(l logging.Level) {
	logging.SetLevel(l, "jabot")
}

func (w *Jabot) GetContact() error {
	return w.client.Roster()
}

func nickName(name string) string {
	if a := strings.SplitN(name, "@", 2); len(a) == 2 {
		return a[0]
	}
	return name
}

func (w *Jabot) updateContacts(contact *Contact) {
	if contact.NickName == "" {
		if contact.Name != "" {
			contact.NickName = contact.Name
		} else {
			contact.NickName = nickName(contact.Jid)
		}
	}
	if cc, ok := w.contacts[contact.Jid]; ok {
		// keep online status
		contact.Online = cc.Online
	}
	w.contacts[contact.Jid] = *contact
}

func (w *Jabot) SendMessage(message string, to string) error {
	if !w.bConnected {
		return errNoConn
	}
	w.lastAct = time.Now()
	chat := xmpp.Chat{Remote: to, Type: "chat", Text: message}
	_, err := w.client.Send(chat)
	return err
}

func (w *Jabot) SendGroupMessage(message string, to string) error {
	if !w.bConnected {
		return errNoConn
	}
	w.lastAct = time.Now()
	chat := xmpp.Chat{Remote: to, Type: "groupchat", Text: message}
	_, err := w.client.Send(chat)
	return err
}
func (w *Jabot) RegisterHandle(cmd string, cmdFunc HandlerFunc) error {
	cmd = strings.ToLower(cmd)
	if _, ok := handlers[cmd]; ok {
		return errHandleExist
	}
	handlers[cmd] = cmdFunc
	return nil
}

func (w *Jabot) getNickName(userName string) string {
	// strip resource
	if a := strings.SplitN(userName, "/", 2); len(a) == 2 {
		userName = a[0]
	}
	if v, ok := w.contacts[userName]; ok {
		return v.NickName
	}

	if a := strings.SplitN(userName, "@", 2); len(a) == 2 {
		return a[0]
	}
	return userName
}

func (w *Jabot) handle(m *xmpp.Chat) error {
	content := strings.TrimSpace(m.Text)
	if content == "" {
		return nil
	}
	from := w.getNickName(m.Remote)
	if from != w.nickName {
		log.Info("[*] ", from, ": ", m.Text)
		cmds := strings.Split(content, ",")
		if len(cmds) == 0 {
			return nil
		}
		cmds[0] = strings.ToLower(cmds[0])
		if cmdFunc, ok := handlers[strings.Trim(cmds[0], " \t")]; ok {
			reply := cmdFunc(cmds[1:])
			if reply != "" {
				if err := w.SendMessage(reply, m.Remote); err != nil {
					return err
				}
				log.Info("[#] ", w.getNickName(w.cfg.Jid), ": ", reply)
			}
		} else {
			if w.auto {
				reply, err := w.getTulingReply(content, m.Remote)
				if err != nil {
					return err
				}

				if err := w.SendMessage(reply, m.Remote); err != nil {
					return err
				}
				log.Info("[#] ", w.nickName, ": ", reply)
			}
		}
	} else {
		switch content {
		case "退下":
			w.auto = false
		case "来人":
			w.auto = true
		default:
			//log.Info("[*#] ", w.nickName, ": ", m.Text)
			cmds := strings.Split(content, ",")
			if len(cmds) == 0 {
				return nil
			}
			cmds[0] = strings.ToLower(cmds[0])
			if cmdFunc, ok := handlers[strings.Trim(cmds[0], " \t")]; ok {
				reply := cmdFunc(cmds[1:])
				if reply != "" {
					if err := w.SendMessage(reply, w.cfg.DefJid); err != nil {
						log.Warning("send myself to defGroup", err)
						return err
					}
					log.Info("[#] ", w.nickName, ": ", reply)
				}
			}
		}
	}
	return nil
}

func (w *Jabot) Dail() error {
	if err := w.dailLoop(0); err != nil {
		w.bConnected = false
		return err
	}
	return nil
}

type rosterItem struct {
	XMLName      xml.Name `xml:"item"`
	Jid          string   `xml:"jid,attr"`
	Name         string   `xml:"name,attr"`
	Subscription string   `xml:"subscription,attr"`
	Group        []string `xml:"group"`
}

func (w *Jabot) dailLoop(timerCnt int) error {
	endT := time.Now().Unix() + int64(timerCnt)
	for timerCnt == 0 || endT > time.Now().Unix() {
		// Recv, and process
		if chat, err := w.client.Recv(); err != nil {
			return err
		} else {
			switch v := chat.(type) {
			case xmpp.Chat:
				if v.Type == "roster" {
					log.Info("roster", v.Roster)
				} else if err := w.handle(&v); err != nil {
					log.Warning("handle chat", err)
				}
			case xmpp.Presence:
				if !w.auto {
					break
				}
				switch v.Type {
				case "subscribe":
					log.Infof("Presence: Approve %s subscription", v.From)
					w.client.ApproveSubscription(v.From)
					w.client.RequestSubscription(v.From)
				case "unsubscribe":
					log.Infof("Presence: Revoke %s subscription", v.From)
					w.client.RevokeSubscription(v.From)
				default:
					log.Infof("Presence: %s %s Type(%s)\n", v.From, v.Show, v.Type)
				}
			case xmpp.Roster, xmpp.Contact:
				log.Info("Roster/Contact:", v)
			case xmpp.IQ:
				// ping ignore
				switch v.QueryName.Space {
				case "jabber:iq:version":
					if v.Type != "get" {
						log.Info(v.QueryName.Space, "type:", v.Type, " with:", v.Query)
						continue
					}
					if err := w.RawVersion(v.To, v.From, v.ID,
						"0.1", runtime.GOOS); err != nil {
						log.Info("RawVersion:", err)
					}
					continue
				case "jabber:iq:last":
					if v.Type != "get" {
						log.Info(v.QueryName.Space, "type:", v.Type, " with:", v.Query)
						continue
					}
					tt := time.Now().Sub(w.lastAct)
					last := int(tt.Seconds())
					//if err := w.RawLastNA(v.To, v.From, v.ID); err != nil {
					if err := w.RawLast(v.To, v.From, v.ID, last); err != nil {
						log.Info("RawLast:", err)
					}
					continue
				case "urn:xmpp:time":
					if v.Type != "get" {
						log.Info(v.QueryName.Space, "type:", v.Type, " with:", v.Query)
						continue
					}
					if err := w.RawIQtime(v.To, v.From, v.ID); err != nil {
						log.Info("RawIQtime:", err)
					}
					continue
				case "jabber:iq:roster":
					var item rosterItem
					if v.Type != "result" && v.Type != "set" {
						// only result and set processed
						log.Info("jabber:iq:roster, type:", v.Type)
						continue
					}
					vv := strings.Split(v.Query, "/>")
					for _, ss := range vv {
						if strings.TrimSpace(ss) == "" {
							continue
						}
						ss += "/>"
						if err := xml.Unmarshal([]byte(ss), &item); err != nil {
							log.Error("unmarshal roster <query>: ", err)
							continue
						} else {
							cc := w.contacts[item.Jid]
							cc.Jid = item.Jid
							cc.Name = item.Name
							cc.Subscription = item.Subscription
							cc.Group = item.Group
							if item.Subscription == "remove" {
								cc.Subscription = ""
								if cc.Jid != "" {
									w.updateContacts(&cc)
								}
								continue
							}
							if cc.Jid != "" {
								w.updateContacts(&cc)
								if cc.Name == "" || cc.NickName == "" {
									// try vCard
								}
								if w.nickName == "" && cc.Jid == w.cfg.Jid {
									if cc.NickName != "" {
										w.nickName = cc.NickName
									} else if cc.Name != "" {
										w.nickName = cc.Name
									} else {
										w.nickName = nickName(cc.Jid)
									}
								}
							}
							if cc.Jid != "" && item.Subscription == "from" &&
								cc.Online {
								log.Infof("roster: Approve %s subscription", cc.Jid)
								//w.client.ApproveSubscription(cc.Jid)
								w.client.RequestSubscription(cc.Jid)
							}
							log.Infof("roster item %s subscription(%s), %v\n",
								item.Jid, item.Subscription, item.Group)
							if v.Type == "set" && item.Subscription == "both" {
								// shall we check presence unavailable
								pr := xmpp.Presence{From: v.To, To: item.Jid,
									Show: "xa"}
								w.client.SendPresence(pr)
							}
						}
					}
					continue
				}
				if v.Type == "result" && v.ID == "c2s1" {
					log.Infof("Got pong from %s to %s\n", v.From, v.To)
				} else {
					log.Infof("Got from %s to %s IQ, tag: (%v), query(%s)\n",
						v.From, v.To, v.QueryName, v.Query)
				}
			default:
				log.Infof("def: %v\n", v)
			}
		}
	}
	return nil
}

func (w *Jabot) Connect() error {
	options := xmpp.Options{User: w.cfg.Jid,
		Password:      w.cfg.Passwd,
		NoTLS:         true,
		Resource:      "jabot",
		Status:        "xa",
		StatusMessage: "I'm gopher jabber",
	}
	/*
		xmpp.DefaultConfig = tls.Config{
			InsecureSkipVerify: true,
		}
	*/
	if talk, err := options.NewClient(); err != nil {
		return err
	} else {
		if w.client != nil {
			w.client.Close()
		}
		w.client = talk
		w.bConnected = true
	}
	w.lastAct = time.Now()
	w.GetContact()
	return nil
}

func (w *Jabot) Close() error {
	if w.client == nil {
		return errNoConn
	}
	w.client.Close()
	w.client = nil
	return nil
}

func NewJabotConn(talk *xmpp.Client) *Jabot {
	rand.Seed(time.Now().Unix())
	randID := strconv.Itoa(rand.Int())

	wx := Jabot{
		cfg:      NewConfig(""),
		resource: "ebot" + randID[2:17],
		client:   talk,
		contacts: make(map[string]Contact),
		auto:     true,
	}
	return &wx

}

func (w *Jabot) IsConnected() bool {
	return w.bConnected
}

//	`%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
func init() {
	var format = logging.MustStringFormatter(
		`%{color}%{time:01-02 15:04:05}  ▶ %{level:.4s} %{color:reset} %{message}`,
	)

	logback := logging.NewLogBackend(os.Stderr, "", 0)
	logfmt := logging.NewBackendFormatter(logback, format)
	logging.SetBackend(logfmt)
}
