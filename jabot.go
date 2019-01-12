package jabot

import (
	"encoding/xml"
	"errors"
	"fmt"
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
	robotName  string
	defJid     string
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
	errHandleExist  = errors.New("命令处理器已经存在")
)
var log = logging.MustGetLogger("jabot")

// HandleFunc type
//	used for RegisterHandle
type HandlerFunc func(args []string) string

var handlers = map[string]HandlerFunc{}
var weGroups = map[string]string{}

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

func (w *Jabot) SetRobotName(name string) {
	w.robotName = name
	//w.auto = false
}

func (w *Jabot) GetContact() error {
	return w.client.Roster()
}

func (w *Jabot) updateContacts(contact Contact) {
	if contact.NickName == "" {
		contact.NickName = contact.Jid
	}
	if cc, ok := w.contacts[contact.Jid]; ok {
		// keep online status
		contact.Online = cc.Online
	}
	w.contacts[contact.Jid] = contact
}

func (w *Jabot) SendMessage(message string, to string) error {
	chat := xmpp.Chat{Remote: to, Type: "chat", Text: message}
	_, err := w.client.Send(chat)
	return err
}

func (w *Jabot) SendGroupMessage(message string, to string) error {
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
	if v, ok := w.contacts[userName]; ok {
		return v.NickName
	}

	if a := strings.SplitN(userName, "@", 2); len(a) == 2 {
		return a[0]
	}
	return userName
}

func (w *Jabot) handle(m *xmpp.Chat) error {
	if m.Remote != w.cfg.Jid {
		//log.Info("[*] ", w.getNickName(m.Remote), ": ", m.Text)
		cmds := strings.Split(strings.TrimSpace(m.Text), ",")
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
				log.Info("[#] ", w.nickName, ": ", reply)
			}
		} else {
			if w.auto {
				reply, err := w.getTulingReply(m.Text, m.Remote)
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
		content := strings.TrimSpace(m.Text)
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
					if err := w.SendMessage(reply, w.defJid); err != nil {
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
					//tt := time.Now().Sub(loginTime)
					//last := int(tt.Seconds())
					//if err := w.RawLast(v.To, v.From, v.ID, last); err != nil {
					if err := w.RawLastNA(v.To, v.From, v.ID); err != nil {
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
							if item.Subscription == "remove" {
								cc.Subscription = ""
								if cc.Jid != "" {
									w.updateContacts(cc)
								}
								continue
							}
							cc.Jid = item.Jid
							cc.Name = item.Name
							cc.Subscription = item.Subscription
							cc.Group = item.Group
							if cc.Jid != "" {
								w.updateContacts(cc)
								if cc.Name == "" || cc.NickName == "" {
									// try vCard
								}
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
	if talk, err := options.NewClient(); err != nil {
		return err
	} else {
		w.client = talk
		w.bConnected = true
	}
	w.GetContact()
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
