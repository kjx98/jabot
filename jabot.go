package jabot

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/kjx98/go-xmpp"
	"github.com/kjx98/golib/to"
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

type vcardTemp struct {
	Name      string `xml:"FN"`
	NickName  string `xml:"NICKNAME"`
	PhotoType string `xml:"PHOTO>TYPE"`
	PhotoImg  []byte `xml:"PHOTO>BINVAL"`
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
type HookFunc func(args string)

var handlers = map[string]HandlerFunc{}
var hookName string
var hookFunc HookFunc

func NewJabot(cfg *Config) (*Jabot, error) {
	rand.Seed(time.Now().Unix())
	randID := to.String(rand.Int())

	wx := Jabot{
		cfg:      *cfg,
		resource: "ebot-" + randID[2:12],
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

func (w *Jabot) RegisterHook(hookStr string, hook HookFunc) {
	if hook == nil {
		hookName = ""
	} else {
		hookName = hookStr
		hookFunc = hook
	}
}

// SetLogLevel
//	logging.Level   from github.com/op/go-logging
func (w *Jabot) SetLogLevel(l logging.Level) {
	logging.SetLevel(l, "jabot")
}

func (w *Jabot) GetRoster() error {
	return w.client.Roster()
}

func nickName(name string) string {
	if a := strings.SplitN(name, "@", 2); len(a) == 2 {
		return a[0]
	}
	return name
}

func getJid(addr string) string {
	if a := strings.SplitN(addr, "/", 2); len(a) == 2 {
		return a[0]
	}
	return addr
}

func getDomain(addr string) string {
	if a := strings.SplitN(getJid(addr), "@", 2); len(a) == 2 {
		return a[1]
	}
	return ""
}

func (w *Jabot) Ping() error {
	if !w.bConnected {
		return errNoConn
	}
	return w.client.PingC2S(w.cfg.Jid, "")
}

func (w *Jabot) AddChat(jid string) error {
	pr := xmpp.Presence{From: w.cfg.Jid, To: jid, Show: "xa"}
	_, err := w.client.SendPresence(pr)
	return err
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

func (w *Jabot) GetContacts() []Contact {
	res := []Contact{}
	for _, cc := range w.contacts {
		if cc.Online {
			res = append(res, cc)
		}
	}
	return res
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
	userName = getJid(userName)
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
	if hookName != "" {
		log.Info("[xH*] ", from, ": ", m.Text)
		//if (hookName == from || from == "") && hookFunc != nil {
		if hookName == from && hookFunc != nil {
			hookFunc(content)
		}
		return nil
	}
	if from != w.nickName {
		log.Info("[x*] ", from, ": ", m.Text)
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
				log.Info("[x#] ", w.getNickName(w.cfg.Jid), ": ", reply)
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
				log.Info("[x#] ", w.nickName, ": ", reply)
			}
		}
	} else {
		switch content {
		case "退下":
			w.auto = false
		case "来人":
			w.auto = true
		default:
			log.Info("[x##] ", w.nickName, ": ", m.Text)
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
					log.Info("[x#] ", w.nickName, ": ", reply)
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
					if getDomain(v.From) != w.cfg.Domain {
						log.Infof("Presence: %s subscription drop", v.From)
						break
					}
					log.Infof("Presence: Approve %s subscription", v.From)
					w.client.ApproveSubscription(v.From)
					w.client.RequestSubscription(v.From)
				case "unsubscribe":
					log.Infof("Presence: Revoke %s subscription", v.From)
					w.client.RevokeSubscription(v.From)
				default:
					jid := getJid(v.From)
					if v.Type == "" {
						// query vcard
						cc := w.contacts[jid]
						//w.updateContacts(&cc)
						if cc.Jid == "" {
							cc.Jid = jid
							cc.Name = w.getNickName(jid)
						}
						cc.Online = true
						w.contacts[jid] = cc
						if cc.Name == "" || cc.NickName == "" {
							w.client.RawInformation(v.To, jid, "vc", "get",
								"<vCard xmlns='vcard-temp'/>")
						}
					} else if v.Type == "unavailble" {
						cc := w.contacts[jid]
						if cc.Jid == jid {
							cc.Online = false
							w.contacts[jid] = cc
						}
					}
					log.Infof("Presence: %s %s Type(%s)\n", v.From, v.Show, v.Type)
				}
			case xmpp.Roster, xmpp.Contact:
				log.Info("Roster/Contact:", v)
			case xmpp.IQ:
				// ping ignore
				var query xml.Name
				if len(v.Query) > 0 && xml.Unmarshal(v.Query, &query) != nil {
					log.Warning("xml.Unmarshal IQ", err)
					continue
				}
				switch query.Space + " " + query.Local {
				case "jabber:iq:version query":
					if v.Type != "get" {
						log.Info("type:", v.Type, " with:", string(v.Query))
						continue
					}
					if err := w.RawVersion(v.To, v.From, v.ID, "0.1",
						runtime.GOOS); err != nil {
						log.Info("RawVersion:", err)
					}
					continue
				case "jabber:iq:last query":
					if v.Type != "get" {
						log.Info("type:", v.Type, " with:", string(v.Query))
						continue
					}
					tt := time.Now().Sub(w.lastAct)
					last := int(tt.Seconds())
					//if err := w.RawLastNA(v.To, v.From, v.ID); err != nil {
					if err := w.RawLast(v.To, v.From, v.ID, last); err != nil {
						log.Info("RawLast:", err)
					}
					continue
				case "urn:xmpp:time time":
					if v.Type != "get" {
						log.Info("type:", v.Type, " with:", string(v.Query))
						continue
					}
					if err := w.RawIQtime(v.To, v.From, v.ID); err != nil {
						log.Info("RawIQtime:", err)
					}
					continue
				case "jabber:iq:roster query":
					type rosterItems struct {
						Items []rosterItem `xml:"item"`
					}
					var roster rosterItems
					if v.Type != "result" && v.Type != "set" {
						// only result and set processed
						log.Info("jabber:iq:roster, type:", v.Type)
						continue
					}
					if err := xml.Unmarshal(v.Query, &roster); err != nil {
						log.Error("unmarshal roster <query>: ", err)
						continue
					}
					for _, item := range roster.Items {
						cc, ok := w.contacts[item.Jid]
						if !ok {
							cc.Jid = item.Jid
						}
						if item.Name != "" {
							cc.Name = item.Name
						}
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
							/*
								// never query vcard here, may loops
								if cc.Name == "" || cc.NickName == "" {
									// try vCard
								}
							*/
						}
						if item.Subscription == "from" && cc.Online {
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
					continue
				case "vcard-temp vCard":
					var it vcardTemp
					if err := xml.Unmarshal(v.Query, &it); err == nil {
						jid := getJid(v.From)
						if v.From == "" {
							// vCard for me
							jid = w.cfg.Jid
							w.nickName = it.NickName
							log.Info("Got nickName of myself:", it.NickName)
						}
						cc := w.contacts[jid]
						if cc.Name == "" || cc.Jid == "" {
							cc.Jid = jid
							cc.Name = it.Name
						}
						if nicN := it.NickName; nicN != "" {
							cc.NickName = nicN
						} else {
							cc.NickName = nickName(jid)
						}
						if it.Name != "" {
							w.updateContacts(&cc)
						}
						pImg, err := base64.StdEncoding.DecodeString(
							string(it.PhotoImg))
						if err != nil {
							log.Info("base64 decode:", err)
						} else if it.PhotoType == "image/png" {
							_ = pImg
							/*
								// PhotoImg is base64 []byte
								if fd, err := os.Create("/tmp/" + jid + ".png"); err == nil {
									fd.Write(pImg)
									fd.Close()
								}
							*/
						}
						log.Infof("Got vCard for %s, FN:%s, Nick:%s/%s",
							v.From, it.Name, it.NickName, cc.NickName)
					} else {
						log.Error("vcard-temp vCard", err)
					}
					continue
				}
				if v.Type == "result" && v.ID == "c2s1" {
					log.Infof("Got pong from %s to %s\n", v.From, v.To)
				} else {
					log.Infof("Got from %s to %s IQ, tag: (%v), query(%s)\n",
						v.From, v.To, query, string(v.Query))
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
		Resource:      w.resource,
		Status:        "xa",
		StatusMessage: "I'm gopher jabber",
	}
	// now could comment out following Skip
	xmpp.DefaultConfig = tls.Config{InsecureSkipVerify: true}
	if talk, err := options.NewClient(); err != nil {
		return err
	} else {
		if w.client != nil {
			w.client.Close()
		}
		w.client = talk
		w.bConnected = true
	}
	w.cfg.Domain = getDomain(w.cfg.Jid)
	w.lastAct = time.Now()
	w.GetRoster()
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
	randID := to.String(rand.Int())

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
		`%{color}%{time:02 15:04:05}  ▶ %{level:.4s} %{color:reset} %{message}`,
	)

	logback := logging.NewLogBackend(os.Stderr, "", 0)
	logfmt := logging.NewBackendFormatter(logback, format)
	logging.SetBackend(logfmt)
}
