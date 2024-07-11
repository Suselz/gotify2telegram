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

func check_all_envs() {
	lvl, exist := os.LookupEnv("LOG_LEVEL")
	if !exist {
		lvl = "info"
	}
	ll, err := log.ParseLevel(lvl)
	if err != nil {
		ll = log.DebugLevel
	}
	log.SetLevel(ll)

	isAllEnvSet := true
	_, exist = os.LookupEnv("GOTIFY_HOST")
	if !exist {
		log.Error("GOTIFY_HOST not set")
		isAllEnvSet = false
	}
	_, exist = os.LookupEnv("GOTIFY_CLIENT_TOKEN")
	if !exist {
		log.Error("Env GOTIFY_CLIENT_TOKEN not set")
		isAllEnvSet = false
	}
	_, exist = os.LookupEnv("TELEGRAM_CHAT_ID")
	if !exist {
		log.Error("Env TELEGRAM_CHAT_ID not set")
		isAllEnvSet = false
	}

	_, exist = os.LookupEnv("TELEGRAM_BOT_TOKEN")
	if !exist {
		log.Error("Env TELEGRAM_BOT_TOKEN not set")
		isAllEnvSet = false
	}
	if !isAllEnvSet {
		log.Fatal("Not all environment variables are set. Exiting.")
	}

	log.Infoln("Env TEMPLATE_PATH not set. Generating default template into file \"./template_default.gotmpl\"")
	_, err = os.Stat(os.Getenv("TEMPLATE_PATH"))
	if os.IsNotExist(err) {
		os.Setenv("TEMPLATE_PATH", "./template_default.gotmpl")
	}
	filepath := os.Getenv("TEMPLATE_PATH")
	if os.IsNotExist(err) {
		defaulttemplate := "{{ .Title }}\n{{ .Date }}\n{{ .Message  }}"
		err := os.WriteFile(filepath, []byte(defaulttemplate), 0666)
		if err != nil {
			log.Errorf("Error while writing default template file: %v\n", err)
		}

	}
}

func main() {
	panic("this should be built as go plugin")
	// FOR DEBUGING
	//--------------------------------------
	// check_all_envs()
	// p := &Plugin{nil, nil, "", "", ""}
	// p.get_websocket_msg(os.Getenv("GOTIFY_HOST"), os.Getenv("GOTIFY_CLIENT_TOKEN"))
	//--------------------------------------
}

// GetGotifyPluginInfo returns gotify plugin info
func GetGotifyPluginInfo() plugin.Info {
	return plugin.Info{
		Version:     "1.0",
		Author:      "Alexandr Dyakonov",
		Name:        "Gotify 2 Telegram",
		Description: "Telegram message forwarder for gotify",
		ModulePath:  "https://github.com/suselz/gotify2telegram",
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
	log.Debugf("Msg:\n-----------------------\n%v\n-----------------------\n", msg)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Errorf("Send request fail: %v\n", err)
		return
	}
	if resp.StatusCode != 200 {
		log.Errorf("Send request fail: %q \n %v\n", resp.Status, resp)
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
	p.chatid = os.Getenv("TELEGRAM_CHAT_ID")
	p.telegram_bot_token = os.Getenv("TELEGRAM_BOT_TOKEN")
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
		tmpl, err := os.ReadFile(os.Getenv("TEMPLATE_PATH"))
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
	log.Debug("Enabling plugin gotify to telegram")
	check_all_envs()
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
