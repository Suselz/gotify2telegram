package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/Masterminds/sprig"
	"github.com/gorilla/websocket"
	"github.com/gotify/plugin-api"
)

type Message struct {
	Organisation string
	Date         string
	Logs         string
	Message      string
	Title        string
}

const TEMPLATE_PATH = "./template.gotmpl"

func main() {
	lvl, ok := os.LookupEnv("LOG_LEVEL")
	// LOG_LEVEL not set, let's default to debug
	if !ok {
		lvl = "debug"
	}
	// parse string, this is built-in feature of logrus
	ll, err := log.ParseLevel(lvl)
	if err != nil {
		ll = log.DebugLevel
	}
	// set global log level
	log.SetLevel(ll)
	// For testing
	p := &Plugin{nil, nil, "", "", ""}
	p.get_websocket_msg(GOTIFY_HOST, GOTIFY_CLIENT_TOKEN) //(os.Getenv("GOTIFY_HOST"), os.Getenv("GOTIFY_CLIENT_TOKEN"))

}

// GetGotifyPluginInfo returns gotify plugin info
func GetGotifyPluginInfo() plugin.Info {
	return plugin.Info{
		Version:     "1.0",
		Author:      "Anh Bui",
		Name:        "Gotify 2 Telegram",
		Description: "Telegram message fowarder for gotify",
		ModulePath:  "https://github.com/anhbh310/gotify2telegram",
	}
}

// Plugin is the plugin instance
type Plugin struct {
	ws                 *websocket.Conn
	msgHandler         plugin.MessageHandler
	chatid             string
	telegram_bot_token string
	gotify_host        string
}

type GotifyMessage struct {
	Id       uint32
	Appid    uint32
	Message  string
	Title    string
	Priority uint32
	Date     string
}

type Payload struct {
	ChatID     string `json:"chat_id"`
	Text       string `json:"text"`
	Parse_mode string `json:"parse_mode"`
}

func (p *Plugin) send_msg_to_telegram(msg string) {

	data := Payload{
		// Fill struct
		ChatID:     p.chatid,
		Text:       msg,
		Parse_mode: "Markdown",
	}
	log.Debugln("Marshaling json")
	payloadBytes, err := json.Marshal(data)
	if err != nil {
		log.Errorf("Error while marshaling json %v\n", err)
		return
	}

	body := bytes.NewReader(payloadBytes)
	log.Debugln("Creating http request")
	req, err := http.NewRequest("POST", "https://api.telegram.org/bot"+p.telegram_bot_token+"/sendMessage", body)
	if err != nil {
		log.Errorf("Error while creating http request %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	log.Infof("Sending request to telegram")
	log.Debugln("Msg:\n", msg)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Errorf("Send request fail: %v\n", err)
		return
	}
	defer resp.Body.Close()
}

func (p *Plugin) connect_websocket() {
	for {
		ws, _, err := websocket.DefaultDialer.Dial(p.gotify_host, nil)
		if err == nil {
			p.ws = ws
			break
		}
		log.Errorf("Cannot connect to websocket: %v\n", err)
		time.Sleep(5000)
	}
}

func (p *Plugin) get_websocket_msg(url string, token string) {
	p.gotify_host = url + "/stream?token=" + token
	p.chatid = chatid                         //os.Getenv("TELEGRAM_CHAT_ID")
	p.telegram_bot_token = telegram_bot_token //os.Getenv("TELEGRAM_BOT_TOKEN")
	log.Debugln("Connecting to gotify")
	go p.connect_websocket()

	for {
		msg := &GotifyMessage{}
		if p.ws == nil {
			time.Sleep(3000)
			continue
		}
		log.Debugln("Wait message and try to reading JSON")
		err := p.ws.ReadJSON(msg)
		if err != nil {
			log.Warnf("Error while reading websocket: %v\n", err)
			p.connect_websocket()
			continue
		}

		log.Debugln("Reading template file")
		tmpl, err := os.ReadFile(TEMPLATE_PATH)
		if err != nil {
			log.Errorf("Error while reading template file: %v\n", err)
		}
		log.Debugln("Rendering message from template")
		t, err := template.New("test").Funcs(sprig.FuncMap()).Parse(string(tmpl))
		if err != nil {
			log.Errorf("Error while rendering template file: %v\n", err)
		}
		log.Debugln("Writing message into buffer")
		var tpl bytes.Buffer
		err = t.Execute(&tpl, msg)
		if err != nil {
			log.Errorf("Error writing message into buffer %v\n", err)
		}
		log.Debugln("Final message:\n", tpl.String())

		p.send_msg_to_telegram(tpl.String())
	}
}

// SetMessageHandler implements plugin.Messenger
// Invoked during initialization
func (p *Plugin) SetMessageHandler(h plugin.MessageHandler) {
	p.msgHandler = h
}

func (p *Plugin) Enable() error {
	go p.get_websocket_msg(os.Getenv("GOTIFY_HOST"), os.Getenv("GOTIFY_CLIENT_TOKEN"))
	return nil
}

// Disable implements plugin.Plugin
func (p *Plugin) Disable() error {
	if p.ws != nil {
		p.ws.Close()
	}
	return nil
}

// NewGotifyPluginInstance creates a plugin instance for a user context.
func NewGotifyPluginInstance(ctx plugin.UserContext) plugin.Plugin {
	return &Plugin{}
}
